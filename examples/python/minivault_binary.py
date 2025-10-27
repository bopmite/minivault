"""MiniVault Binary Protocol Client for Python"""

import socket
import struct
import json
from typing import Optional, Any, Dict

OP_GET = 0x01
OP_SET = 0x02
OP_DELETE = 0x03
OP_HEALTH = 0x05
OP_AUTH = 0x06

STATUS_SUCCESS = 0x00
STATUS_ERROR = 0xFF


class MiniVaultBinary:
    def __init__(self, address: str, api_key: Optional[str] = None, timeout: int = 5):
        host, port = address.split(':')
        self.host = host
        self.port = int(port)
        self.api_key = api_key
        self.timeout = timeout

    def _connect(self) -> socket.socket:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(self.timeout)
        sock.connect((self.host, self.port))
        return sock

    def _send_request(self, sock: socket.socket, request: bytes) -> bytes:
        sock.sendall(request)

        # Read response header (5 bytes)
        header = sock.recv(5)
        if len(header) < 5:
            raise Exception("Incomplete response header")

        status = header[0]
        data_len = struct.unpack('<I', header[1:5])[0]

        if status != STATUS_SUCCESS:
            raise Exception(f"Server error: status=0x{status:02x}")

        # Read response data
        data = b''
        remaining = data_len
        while remaining > 0:
            chunk = sock.recv(min(remaining, 8192))
            if not chunk:
                raise Exception("Connection closed while reading data")
            data += chunk
            remaining -= len(chunk)

        return data

    def _authenticate(self, sock: socket.socket):
        if not self.api_key:
            return

        key_bytes = self.api_key.encode('utf-8')
        request = struct.pack('<BH', OP_AUTH, len(key_bytes)) + key_bytes
        self._send_request(sock, request)

    def _execute_operation(self, op: int, key: str, value: Optional[bytes] = None) -> bytes:
        sock = self._connect()
        try:
            self._authenticate(sock)

            key_bytes = key.encode('utf-8')

            if op in (OP_GET, OP_DELETE, OP_HEALTH):
                request = struct.pack('<BH', op, len(key_bytes)) + key_bytes
            elif op == OP_SET and value is not None:
                request = (
                    struct.pack('<BH', op, len(key_bytes)) +
                    key_bytes +
                    struct.pack('<IB', len(value), 0) +  # value length + not compressed
                    value
                )
            else:
                raise Exception("Invalid operation")

            return self._send_request(sock, request)
        finally:
            sock.close()

    def get(self, key: str) -> Optional[bytes]:
        """Get raw bytes for a key"""
        try:
            data = self._execute_operation(OP_GET, key)
            return data if len(data) > 0 else None
        except Exception as e:
            print(f"GET error for {key}: {e}")
            return None

    def get_json(self, key: str) -> Optional[Any]:
        """Get and deserialize JSON"""
        data = self.get(key)
        if data is None:
            return None
        try:
            return json.loads(data.decode('utf-8'))
        except Exception as e:
            print(f"Failed to parse JSON for {key}: {e}")
            return None

    def set(self, key: str, value: bytes) -> bool:
        """Set raw bytes for a key"""
        try:
            self._execute_operation(OP_SET, key, value)
            return True
        except Exception as e:
            print(f"SET error for {key}: {e}")
            return False

    def set_json(self, key: str, value: Any) -> bool:
        """Serialize and store JSON"""
        data = json.dumps(value).encode('utf-8')
        return self.set(key, data)

    def delete(self, key: str) -> bool:
        """Delete a key"""
        try:
            self._execute_operation(OP_DELETE, key)
            return True
        except Exception as e:
            print(f"DELETE error for {key}: {e}")
            return False

    def health(self) -> Optional[Dict[str, Any]]:
        """Get cluster health"""
        try:
            data = self._execute_operation(OP_HEALTH, "health")
            return json.loads(data.decode('utf-8'))
        except Exception as e:
            print(f"Health check error: {e}")
            return None

    def exists(self, key: str) -> bool:
        """Check if key exists"""
        return self.get(key) is not None
