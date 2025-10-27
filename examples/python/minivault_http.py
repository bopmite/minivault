"""MiniVault HTTP Client for Python"""

import requests
from typing import Optional, Any, Dict
import json


class MiniVault:
    def __init__(self, base_url: str, api_key: Optional[str] = None, timeout: int = 5):
        self.base_url = base_url.rstrip('/')
        self.api_key = api_key
        self.timeout = timeout
        self.session = requests.Session()

    def get(self, key: str) -> Optional[Any]:
        """Get a value (automatically unwraps from JSON response)"""
        try:
            url = f"{self.base_url}/{key}"
            response = self.session.get(url, timeout=self.timeout)

            if response.status_code == 404:
                return None

            response.raise_for_status()
            result = response.json()

            if result.get('success', False):
                return result.get('data')

            return None
        except Exception as e:
            print(f"GET error for {key}: {e}")
            return None

    def get_json(self, key: str) -> Optional[Any]:
        """Get and deserialize JSON"""
        return self.get(key)

    def set(self, key: str, value: Any) -> bool:
        """Set a value (automatically wraps in JSON request)"""
        try:
            url = f"{self.base_url}/{key}"
            headers = {'Content-Type': 'application/json'}

            if self.api_key:
                headers['Authorization'] = f'Bearer {self.api_key}'

            payload = {'value': value}
            response = self.session.put(url, json=payload, headers=headers, timeout=self.timeout)
            response.raise_for_status()

            result = response.json()
            return result.get('success', False)
        except Exception as e:
            print(f"SET error for {key}: {e}")
            return False

    def set_json(self, key: str, value: Any) -> bool:
        """Serialize and store JSON"""
        return self.set(key, value)

    def delete(self, key: str) -> bool:
        """Delete a key"""
        try:
            url = f"{self.base_url}/{key}"
            headers = {}

            if self.api_key:
                headers['Authorization'] = f'Bearer {self.api_key}'

            response = self.session.delete(url, headers=headers, timeout=self.timeout)
            response.raise_for_status()

            result = response.json()
            return result.get('success', False)
        except Exception as e:
            print(f"DELETE error for {key}: {e}")
            return False

    def exists(self, key: str) -> bool:
        """Check if key exists"""
        return self.get(key) is not None

    def health(self) -> Optional[Dict[str, Any]]:
        """Get cluster health"""
        try:
            url = f"{self.base_url}/health"
            response = self.session.get(url, timeout=self.timeout)
            response.raise_for_status()
            return response.json()
        except Exception as e:
            print(f"Health check error: {e}")
            return None

    def mget(self, keys: list[str]) -> Dict[str, Any]:
        """Parallel get multiple keys"""
        from concurrent.futures import ThreadPoolExecutor

        results = {}
        with ThreadPoolExecutor(max_workers=min(len(keys), 10)) as executor:
            futures = {executor.submit(self.get, key): key for key in keys}
            for future in futures:
                key = futures[future]
                data = future.result()
                if data is not None:
                    results[key] = data

        return results

    def mset(self, entries: Dict[str, Any]) -> bool:
        """Parallel set multiple key-value pairs"""
        from concurrent.futures import ThreadPoolExecutor

        with ThreadPoolExecutor(max_workers=min(len(entries), 10)) as executor:
            futures = [executor.submit(self.set, key, value) for key, value in entries.items()]
            results = [f.result() for f in futures]

        return all(results)
