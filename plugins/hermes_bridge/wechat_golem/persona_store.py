"""微信多人格的独立本地存储。

仅使用 Python 标准库，不依赖 Hermes 或 HTTP 框架。
"""

from __future__ import annotations

import base64
import errno
import hashlib
import json
import os
import re
import stat
import tempfile
import threading
import time
from contextlib import contextmanager
from dataclasses import dataclass
from typing import Any, Dict, List, Optional, Tuple

DEFAULT_PERSONA_ID = "default"
PERSONA_STORE_PROTOCOL = 1
PERSONA_MAX_BYTES = 64 * 1024
BINDINGS_MAX_BYTES = 1024 * 1024
BINDINGS_VERSION = 1
BINDINGS_NAME = "session_bindings.json"
BINDINGS_LOCK_NAME = ".session_bindings.lock"
PERSONA_ID_RE = re.compile(r"^[^\W_][\w-]{0,63}$", re.UNICODE)
SESSION_KEY_RE = re.compile(r"^(?:chatroom|private):[^\x00\r\n]{1,256}$")


class PersonaStoreError(Exception):
    """人格存储错误基类。"""

    def __init__(self, message: str, *, signature: Any = None):
        super().__init__(message)
        self.signature = signature


class InvalidPersonaID(PersonaStoreError):
    """人格 ID 不合法。"""


class InvalidSessionKey(PersonaStoreError):
    """会话键不是桥提供的稳定键。"""


class PersonaNotFound(PersonaStoreError):
    """人格不存在。"""


class PersonaUnavailable(PersonaStoreError):
    """人格文件存在但当前不可安全读取。"""


class PersonaAlreadyExists(PersonaStoreError):
    """创建的人格 ID 已存在。"""


class PersonaConflict(PersonaStoreError):
    """If-Match 与当前 ETag 不一致。"""


class PersonaDeleteForbidden(PersonaStoreError):
    """人格因 default 或仍被绑定而不可删除。"""


class BindingsCorrupt(PersonaStoreError):
    """绑定文件损坏；读取可回退，所有写操作冻结。"""


class StoreTypeError(PersonaStoreError):
    """目录或文件不是非链接、非重解析点普通类型。"""


class StoreLimitError(PersonaStoreError):
    """内容超过存储协议上限。"""


class StoreLockTimeout(PersonaStoreError):
    """等待跨进程绑定锁超时。"""


@dataclass(frozen=True)
class PersonaRecord:
    """完整人格记录。"""

    persona_id: str
    content: str
    etag: str
    revision: str
    size_bytes: int
    updated_at_ns: int


@dataclass(frozen=True)
class PersonaSummary:
    """人格列表摘要。"""

    persona_id: str
    etag: str
    revision: str
    size_bytes: int
    updated_at_ns: int
    binding_count: int
    is_default: bool
    available: bool = True


@dataclass(frozen=True)
class BindingRecord:
    """单条会话绑定。"""

    session_key: str
    persona_id: str
    updated_at: str


def is_valid_persona_id(persona_id: str) -> bool:
    return bool(PERSONA_ID_RE.fullmatch(str(persona_id or "")))


def is_stable_session_key(session_key: str) -> bool:
    return bool(SESSION_KEY_RE.fullmatch(str(session_key or "").strip()))


def encode_binding_id(session_key: str) -> str:
    value = str(session_key or "").strip()
    return base64.urlsafe_b64encode(value.encode("utf-8")).decode("ascii").rstrip("=")


def decode_binding_id(binding_id: str) -> str:
    raw = str(binding_id or "").strip()
    if not raw or len(raw) > 512:
        raise InvalidSessionKey("绑定标识无效")
    padding = "=" * ((4 - len(raw) % 4) % 4)
    try:
        session_key = base64.urlsafe_b64decode(raw + padding).decode("utf-8")
    except (ValueError, UnicodeDecodeError) as exc:
        raise InvalidSessionKey("绑定标识无法解码") from exc
    if not is_stable_session_key(session_key):
        raise InvalidSessionKey("绑定标识不是有效的微信稳定会话键")
    return session_key


def _is_reparse_point(info: os.stat_result) -> bool:
    attrs = getattr(info, "st_file_attributes", 0)
    marker = getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x0400)
    return bool(attrs & marker)


def _signature(info: os.stat_result) -> Tuple[int, ...]:
    return (
        info.st_dev,
        info.st_ino,
        info.st_ctime_ns,
        info.st_mtime_ns,
        info.st_size,
        info.st_mode,
    )


def _revision(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def _etag(revision: str) -> str:
    return f'"sha256-{revision}"'


def _updated_at() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


class PersonaStore:
    """管理人格文件与会话绑定，支持多进程安全更新。"""

    def __init__(
        self,
        hermes_home: Optional[str] = None,
        *,
        root: Optional[str] = None,
        lock_timeout: float = 5.0,
    ) -> None:
        home = hermes_home
        if home is None:
            home = os.environ.get("HERMES_HOME") or os.path.expanduser("~/.hermes")
        self.hermes_home = os.path.abspath(os.path.expanduser(str(home)))
        self.root = (
            os.path.abspath(os.path.expanduser(str(root)))
            if root is not None
            else os.path.join(self.hermes_home, "wechat_personas")
        )
        self.lock_timeout = max(0.0, float(lock_timeout))
        self._thread_lock = threading.RLock()
        self._persona_cache: Dict[str, Tuple[Any, PersonaRecord]] = {}
        self._bindings_cache: Optional[Tuple[Any, Dict[str, Dict[str, str]], str]] = None

    def validate_persona_id(self, persona_id: str) -> str:
        value = str(persona_id or "").strip()
        if not is_valid_persona_id(value):
            raise InvalidPersonaID(
                "人格 ID 首字符必须是 Unicode 字母或数字，其余只允许 Unicode 字母、数字、下划线和连字符，总长最多 64 字符"
            )
        return value

    def validate_session_key(self, session_key: str) -> str:
        value = str(session_key or "").strip()
        if not is_stable_session_key(value):
            raise InvalidSessionKey("缺少有效的微信稳定会话键")
        return value

    def _checked_root(self, *, create: bool = False) -> Tuple[str, Any]:
        if create:
            parent = os.path.dirname(self.root)
            if not os.path.isdir(parent):
                os.makedirs(parent, mode=0o700, exist_ok=True)
            try:
                os.mkdir(self.root, 0o700)
            except FileExistsError:
                pass
        try:
            info = os.lstat(self.root)
        except FileNotFoundError:
            raise
        if (
            stat.S_ISLNK(info.st_mode)
            or _is_reparse_point(info)
            or not stat.S_ISDIR(info.st_mode)
        ):
            raise StoreTypeError("wechat_personas 必须是非链接、非重解析点的普通目录")
        return self.root, _signature(info)

    def _path(self, persona_id: str) -> str:
        return os.path.join(self.root, f"{self.validate_persona_id(persona_id)}.md")

    def _read_regular(self, path: str, limit: int, *, allow_empty: bool) -> Tuple[bytes, Any]:
        try:
            listed = os.lstat(path)
        except FileNotFoundError:
            raise
        if (
            stat.S_ISLNK(listed.st_mode)
            or _is_reparse_point(listed)
            or not stat.S_ISREG(listed.st_mode)
        ):
            raise StoreTypeError("存储文件必须是非链接、非重解析点的普通文件")
        if listed.st_size > limit or (not allow_empty and listed.st_size <= 0):
            raise StoreLimitError(f"文件大小非法: {listed.st_size}")
        flags = os.O_RDONLY | getattr(os, "O_BINARY", 0)
        if hasattr(os, "O_NOFOLLOW"):
            flags |= os.O_NOFOLLOW
        fd = os.open(path, flags)
        try:
            opened = os.fstat(fd)
            if not stat.S_ISREG(opened.st_mode) or _is_reparse_point(opened):
                raise StoreTypeError("打开后的存储对象不是普通文件")
            chunks: List[bytes] = []
            total = 0
            while total <= limit:
                chunk = os.read(fd, min(64 * 1024, limit + 1 - total))
                if not chunk:
                    break
                chunks.append(chunk)
                total += len(chunk)
            if total > limit or (not allow_empty and total <= 0):
                raise StoreLimitError(f"文件内容大小非法: {total}")
            return b"".join(chunks), _signature(opened)
        finally:
            os.close(fd)

    def _atomic_write(self, path: str, raw: bytes, *, prefix: str) -> None:
        root, _ = self._checked_root(create=True)
        fd = -1
        temp_path = ""
        try:
            fd, temp_path = tempfile.mkstemp(prefix=prefix, suffix=".tmp", dir=root)
            try:
                os.chmod(temp_path, 0o600)
            except OSError:
                pass
            with os.fdopen(fd, "wb") as stream:
                fd = -1
                stream.write(raw)
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(temp_path, path)
            temp_path = ""
            try:
                if hasattr(os, "O_DIRECTORY"):
                    dir_fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY)
                    try:
                        os.fsync(dir_fd)
                    finally:
                        os.close(dir_fd)
            except OSError:
                pass
        finally:
            if fd >= 0:
                os.close(fd)
            if temp_path:
                try:
                    os.remove(temp_path)
                except OSError:
                    pass

    def read_persona(self, persona_id: str) -> PersonaRecord:
        persona_id = self.validate_persona_id(persona_id)
        try:
            root, root_sig = self._checked_root()
            path = os.path.join(root, f"{persona_id}.md")
            listed = os.lstat(path)
        except FileNotFoundError as exc:
            raise PersonaNotFound(f"人格 {persona_id} 不存在") from exc
        except PersonaStoreError:
            raise
        except OSError as exc:
            raise PersonaUnavailable(f"人格 {persona_id} 当前不可用：{exc}") from exc
        sig = root_sig + _signature(listed)
        with self._thread_lock:
            cached = self._persona_cache.get(persona_id)
            if cached and cached[0] == sig:
                return cached[1]
        try:
            raw, opened_sig = self._read_regular(path, PERSONA_MAX_BYTES, allow_empty=False)
            content = raw.decode("utf-8").strip()
            if not content:
                raise PersonaUnavailable(f"人格 {persona_id} 文件为空")
        except PersonaStoreError:
            raise
        except FileNotFoundError as exc:
            raise PersonaNotFound(f"人格 {persona_id} 不存在") from exc
        except (OSError, UnicodeDecodeError) as exc:
            raise PersonaUnavailable(f"读取人格 {persona_id} 失败：{exc}") from exc
        revision = _revision(raw)
        record = PersonaRecord(
            persona_id=persona_id,
            content=content,
            etag=_etag(revision),
            revision=revision,
            size_bytes=len(raw),
            updated_at_ns=opened_sig[3],
        )
        with self._thread_lock:
            self._persona_cache[persona_id] = (root_sig + opened_sig, record)
        return record

    def load_persona(self, persona_id: str) -> str:
        return self.read_persona(persona_id).content

    def list_personas(self, *, include_unavailable: bool = False) -> List[PersonaSummary]:
        try:
            root, _ = self._checked_root()
            names = os.listdir(root)
        except FileNotFoundError:
            return []
        bindings, _ = self.read_bindings()
        counts: Dict[str, int] = {}
        for item in bindings.values():
            target = item["persona_id"]
            counts[target] = counts.get(target, 0) + 1
        out: List[PersonaSummary] = []
        for name in names:
            if not name.endswith(".md"):
                continue
            persona_id = name[:-3]
            if not is_valid_persona_id(persona_id):
                continue
            try:
                record = self.read_persona(persona_id)
            except (PersonaStoreError, OSError):
                if not include_unavailable:
                    continue
                updated_at_ns = 0
                size_bytes = 0
                try:
                    listed = os.lstat(os.path.join(root, name))
                    updated_at_ns = listed.st_mtime_ns
                    size_bytes = listed.st_size
                except OSError:
                    pass
                out.append(
                    PersonaSummary(
                        persona_id=persona_id,
                        etag="",
                        revision="",
                        size_bytes=size_bytes,
                        updated_at_ns=updated_at_ns,
                        binding_count=counts.get(persona_id, 0),
                        is_default=persona_id == DEFAULT_PERSONA_ID,
                        available=False,
                    )
                )
                continue
            out.append(
                PersonaSummary(
                    persona_id=persona_id,
                    etag=record.etag,
                    revision=record.revision,
                    size_bytes=record.size_bytes,
                    updated_at_ns=record.updated_at_ns,
                    binding_count=counts.get(persona_id, 0),
                    is_default=persona_id == DEFAULT_PERSONA_ID,
                    available=True,
                )
            )
        return sorted(out, key=lambda item: item.persona_id)

    def list_persona_ids(self) -> List[str]:
        return [item.persona_id for item in self.list_personas()]

    def _encode_persona(self, content: str) -> bytes:
        value = str(content or "").strip()
        if not value:
            raise PersonaUnavailable("人格内容不能为空")
        raw = (value + "\n").encode("utf-8")
        if len(raw) > PERSONA_MAX_BYTES:
            raise StoreLimitError("人格内容超过 64 KiB UTF-8 上限")
        return raw

    def create_persona(self, persona_id: str, content: str) -> PersonaRecord:
        persona_id = self.validate_persona_id(persona_id)
        raw = self._encode_persona(content)
        path = self._path(persona_id)
        with self._thread_lock:
            with self._bindings_file_lock():
                try:
                    os.lstat(path)
                except FileNotFoundError:
                    pass
                else:
                    raise PersonaAlreadyExists(f"人格 {persona_id} 已存在")
                self._atomic_write(path, raw, prefix=f".{persona_id}-")
                self._persona_cache.pop(persona_id, None)
        return self.read_persona(persona_id)

    def update_persona(
        self, persona_id: str, content: str, *, if_match: str
    ) -> PersonaRecord:
        persona_id = self.validate_persona_id(persona_id)
        raw = self._encode_persona(content)
        if not str(if_match or "").strip():
            raise PersonaConflict("更新人格必须提供 If-Match")
        with self._thread_lock:
            with self._bindings_file_lock():
                current = self.read_persona(persona_id)
                if if_match not in (current.etag, current.revision, "*"):
                    raise PersonaConflict("人格已被其他操作更新，请刷新后重试")
                self._atomic_write(self._path(persona_id), raw, prefix=f".{persona_id}-")
                self._persona_cache.pop(persona_id, None)
        return self.read_persona(persona_id)

    def delete_persona(self, persona_id: str, *, if_match: Optional[str] = None) -> None:
        persona_id = self.validate_persona_id(persona_id)
        if persona_id == DEFAULT_PERSONA_ID:
            raise PersonaDeleteForbidden("default 人格不可删除")
        with self._thread_lock:
            with self._bindings_file_lock():
                _, bindings, error = self._read_bindings_disk()
                if error:
                    raise BindingsCorrupt(
                        "人格绑定文件损坏或版本不兼容，已冻结删除", signature=error[0]
                    )
                used_by = sorted(
                    key for key, item in bindings.items() if item["persona_id"] == persona_id
                )
                if used_by:
                    raise PersonaDeleteForbidden(
                        f"人格仍被 {len(used_by)} 个会话绑定，不能删除"
                    )
                current = None
                path = self._path(persona_id)
                try:
                    listed = os.lstat(path)
                    if (
                        stat.S_ISLNK(listed.st_mode)
                        or _is_reparse_point(listed)
                        or not stat.S_ISREG(listed.st_mode)
                    ):
                        raise StoreTypeError("存储文件必须是非链接、非重解析点的普通文件")
                except FileNotFoundError as exc:
                    raise PersonaNotFound(f"人格 {persona_id} 不存在") from exc
                try:
                    current = self.read_persona(persona_id)
                except PersonaNotFound:
                    raise
                except PersonaStoreError:
                    current = None
                if if_match:
                    if current is None or if_match not in (
                        current.etag,
                        current.revision,
                        "*",
                    ):
                        raise PersonaConflict("人格已被其他操作更新，请刷新后重试")
                os.remove(path)
                self._persona_cache.pop(persona_id, None)

    def _normalize_bindings(self, raw: Any) -> Dict[str, Dict[str, str]]:
        if not isinstance(raw, dict):
            raise ValueError("顶层必须是对象")
        if raw.get("version") != BINDINGS_VERSION:
            raise ValueError(f"不支持的 version: {raw.get('version')!r}")
        source = raw.get("bindings")
        if not isinstance(source, dict):
            raise ValueError("bindings 必须是对象")
        out: Dict[str, Dict[str, str]] = {}
        for session_key, item in source.items():
            if not isinstance(session_key, str) or not is_stable_session_key(session_key):
                raise ValueError(f"非法 session_key: {session_key!r}")
            if not isinstance(item, dict):
                raise ValueError(f"绑定项必须是对象: {session_key}")
            persona_id = item.get("persona_id")
            updated_at = item.get("updated_at")
            if not isinstance(persona_id, str) or not is_valid_persona_id(persona_id):
                raise ValueError(f"非法 persona_id: {session_key}")
            if not isinstance(updated_at, str) or not updated_at.strip():
                raise ValueError(f"updated_at 缺失: {session_key}")
            out[session_key] = {
                "persona_id": persona_id,
                "updated_at": updated_at.strip(),
            }
        return out

    def _bindings_signature(self) -> Any:
        try:
            root, root_sig = self._checked_root()
            info = os.lstat(os.path.join(root, BINDINGS_NAME))
            return root_sig + _signature(info)
        except FileNotFoundError:
            return None
        except OSError as exc:
            return ("error", exc.errno, str(exc))

    def _read_bindings_disk(
        self,
    ) -> Tuple[Any, Dict[str, Dict[str, str]], Optional[Tuple[Any, str]]]:
        try:
            root, root_sig = self._checked_root()
        except FileNotFoundError:
            return None, {}, None
        except (OSError, PersonaStoreError) as exc:
            signature = ("root", getattr(exc, "errno", None), str(exc))
            return signature, {}, (signature, str(exc))
        path = os.path.join(root, BINDINGS_NAME)
        try:
            raw, file_sig = self._read_regular(path, BINDINGS_MAX_BYTES, allow_empty=False)
            signature = root_sig + file_sig
            bindings = self._normalize_bindings(json.loads(raw.decode("utf-8")))
            return signature, bindings, None
        except FileNotFoundError:
            return None, {}, None
        except Exception as exc:
            try:
                info = os.lstat(path)
                signature = root_sig + _signature(info)
            except OSError:
                signature = ("bindings", str(exc))
            return signature, {}, (signature, str(exc))

    def read_bindings(
        self, *, force: bool = False
    ) -> Tuple[Dict[str, Dict[str, str]], str]:
        """读取绑定；损坏时返回空绑定与错误文本，不向读取链抛错。"""
        signature = self._bindings_signature()
        with self._thread_lock:
            cached = self._bindings_cache
            if not force and cached is not None and cached[0] == signature:
                return {key: dict(value) for key, value in cached[1].items()}, cached[2]
            actual, bindings, error = self._read_bindings_disk()
            error_text = error[1] if error else ""
            self._bindings_cache = (actual, bindings, error_text)
            return {key: dict(value) for key, value in bindings.items()}, error_text

    def list_bindings(self) -> List[BindingRecord]:
        bindings, error = self.read_bindings()
        if error:
            return []
        return [
            BindingRecord(key, item["persona_id"], item["updated_at"])
            for key, item in sorted(bindings.items())
        ]

    def get_binding(self, session_key: str) -> Optional[BindingRecord]:
        session_key = self.validate_session_key(session_key)
        bindings, error = self.read_bindings()
        if error:
            return None
        item = bindings.get(session_key)
        if not item:
            return None
        return BindingRecord(session_key, item["persona_id"], item["updated_at"])

    @contextmanager
    def _bindings_file_lock(self):
        root, _ = self._checked_root(create=True)
        path = os.path.join(root, BINDINGS_LOCK_NAME)
        flags = os.O_RDWR | os.O_CREAT | getattr(os, "O_BINARY", 0)
        if hasattr(os, "O_NOFOLLOW"):
            flags |= os.O_NOFOLLOW
        fd = os.open(path, flags, 0o600)
        locked = False
        try:
            opened = os.fstat(fd)
            if not stat.S_ISREG(opened.st_mode) or _is_reparse_point(opened):
                raise StoreTypeError("人格绑定锁必须是普通文件")
            if opened.st_size < 1:
                os.write(fd, b"\0")
                os.fsync(fd)
            deadline = time.monotonic() + self.lock_timeout
            while True:
                try:
                    os.lseek(fd, 0, os.SEEK_SET)
                    if os.name == "nt":
                        import msvcrt

                        msvcrt.locking(fd, msvcrt.LK_NBLCK, 1)
                    else:
                        import fcntl

                        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    locked = True
                    break
                except OSError as exc:
                    if exc.errno not in (errno.EACCES, errno.EAGAIN, errno.EDEADLK):
                        raise
                    if time.monotonic() >= deadline:
                        raise StoreLockTimeout("人格绑定文件正被其他进程更新，请稍后重试")
                    time.sleep(0.05)
            yield root
        finally:
            if locked:
                try:
                    os.lseek(fd, 0, os.SEEK_SET)
                    if os.name == "nt":
                        import msvcrt

                        msvcrt.locking(fd, msvcrt.LK_UNLCK, 1)
                    else:
                        import fcntl

                        fcntl.flock(fd, fcntl.LOCK_UN)
                except OSError:
                    pass
            os.close(fd)

    def _write_bindings_locked(self, bindings: Dict[str, Dict[str, str]]) -> None:
        payload = {"version": BINDINGS_VERSION, "bindings": bindings}
        raw = (json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode(
            "utf-8"
        )
        if len(raw) > BINDINGS_MAX_BYTES:
            raise StoreLimitError("人格绑定文件更新后超过 1 MiB")
        path = os.path.join(self.root, BINDINGS_NAME)
        self._atomic_write(path, raw, prefix=".session_bindings-")
        signature, actual, error = self._read_bindings_disk()
        if error:
            self._bindings_cache = (signature, {}, error[1])
            raise BindingsCorrupt(f"绑定已经写入，但重新读取失败：{error[1]}")
        self._bindings_cache = (signature, actual, "")

    def write_binding(self, session_key: str, persona_id: Optional[str]) -> None:
        """设置或删除绑定；切换目标会在共享锁内再次验证。"""
        session_key = self.validate_session_key(session_key)
        if persona_id is not None:
            persona_id = self.validate_persona_id(persona_id)
            if persona_id == DEFAULT_PERSONA_ID:
                persona_id = None
        with self._thread_lock:
            with self._bindings_file_lock():
                signature, bindings, error = self._read_bindings_disk()
                if error:
                    self._bindings_cache = (signature, {}, error[1])
                    raise BindingsCorrupt(
                        "人格绑定文件损坏或版本不兼容，已拒绝覆盖",
                        signature=signature,
                    )
                # 必须在绑定锁内重新读目标，和安全删除共用同一临界区。
                if persona_id is not None:
                    self.read_persona(persona_id)
                updated = {key: dict(value) for key, value in bindings.items()}
                if persona_id is None:
                    if session_key not in updated:
                        self._bindings_cache = (signature, bindings, "")
                        return
                    updated.pop(session_key, None)
                else:
                    updated[session_key] = {
                        "persona_id": persona_id,
                        "updated_at": _updated_at(),
                    }
                self._write_bindings_locked(updated)

    def unbind(self, session_key: str) -> bool:
        session_key = self.validate_session_key(session_key)
        existed = self.get_binding(session_key) is not None
        self.write_binding(session_key, None)
        return existed

    def resolve(self, session_key: str) -> Dict[str, Any]:
        """解析会话实际人格；绑定损坏时忽略显式绑定。"""
        session_key = str(session_key or "").strip()
        bindings, error = self.read_bindings()
        item = bindings.get(session_key) if is_stable_session_key(session_key) else None
        configured = str((item or {}).get("persona_id") or "")
        requested = configured or DEFAULT_PERSONA_ID
        try:
            record = self.read_persona(requested)
            return {
                "configured": configured,
                "effective": requested,
                "content": record.content,
                "status": "ok",
                "bindings_error": error,
                "binding_ignored": bool(error),
            }
        except PersonaStoreError:
            if requested != DEFAULT_PERSONA_ID:
                try:
                    fallback = self.read_persona(DEFAULT_PERSONA_ID)
                    return {
                        "configured": configured,
                        "effective": DEFAULT_PERSONA_ID,
                        "content": fallback.content,
                        "status": "fallback_default",
                        "bindings_error": error,
                        "binding_ignored": bool(error),
                    }
                except PersonaStoreError:
                    pass
        return {
            "configured": configured,
            "effective": "",
            "content": "",
            "status": "soul_only",
            "bindings_error": error,
            "binding_ignored": bool(error),
        }

    def resolve_active(self, session_key: str) -> Tuple[str, str]:
        state = self.resolve(session_key)
        return str(state["effective"] or DEFAULT_PERSONA_ID), str(state["content"] or "")
