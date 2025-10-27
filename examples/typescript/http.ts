export interface MiniVaultHealth {
  status: string;
  uptime_seconds: number;
  cache_items: number;
  cache_size_mb: number;
  storage_size_mb: number;
  goroutines: number;
  memory_mb: number;
}

export interface MiniVaultOptions {
  baseUrl: string;
  apiKey?: string;
  timeout?: number;
}

export class MiniVault {
  private baseUrl: string;
  private apiKey?: string;
  private timeout: number;

  constructor(options: MiniVaultOptions | string, apiKey?: string) {
    if (typeof options === 'string') {
      this.baseUrl = options;
      this.apiKey = apiKey;
      this.timeout = 5000;
    } else {
      this.baseUrl = options.baseUrl;
      this.apiKey = options.apiKey;
      this.timeout = options.timeout || 5000;
    }
  }

  async get<T = any>(key: string): Promise<T | null> {
    try {
      const url = `${this.baseUrl}/${key}`;
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), this.timeout);

      const headers: HeadersInit = {};
      if (this.apiKey) {
        headers['Authorization'] = `Bearer ${this.apiKey}`;
      }

      const response = await fetch(url, {
        method: 'GET',
        headers,
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      if (response.status === 404) {
        return null;
      }

      if (!response.ok) {
        return null;
      }

      const result = await response.json();
      if (result.success && result.data !== undefined) {
        return result.data as T;
      }

      return null;
    } catch (error) {
      return null;
    }
  }

  async set(key: string, value: any): Promise<boolean> {
    try {
      const url = `${this.baseUrl}/${key}`;
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), this.timeout);

      const headers: HeadersInit = {
        'Content-Type': 'application/json',
      };

      if (this.apiKey) {
        headers['Authorization'] = `Bearer ${this.apiKey}`;
      }

      const response = await fetch(url, {
        method: 'PUT',
        headers,
        body: JSON.stringify({ value }),
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        return false;
      }

      const result = await response.json();
      return result.success === true;
    } catch (error) {
      return false;
    }
  }

  async delete(key: string): Promise<boolean> {
    try {
      const url = `${this.baseUrl}/${key}`;
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), this.timeout);

      const headers: HeadersInit = {};
      if (this.apiKey) {
        headers['Authorization'] = `Bearer ${this.apiKey}`;
      }

      const response = await fetch(url, {
        method: 'DELETE',
        headers,
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      const result = await response.json();
      return result.success === true;
    } catch (error) {
      return false;
    }
  }

  async mget<T = any>(keys: string[]): Promise<Array<T | null>> {
    const promises = keys.map(key => this.get<T>(key));
    return Promise.all(promises);
  }

  async mset(entries: Array<{ key: string; value: any }>): Promise<boolean[]> {
    const promises = entries.map(({ key, value }) => this.set(key, value));
    return Promise.all(promises);
  }

  async exists(key: string): Promise<boolean> {
    const data = await this.get(key);
    return data !== null;
  }

  async health(): Promise<MiniVaultHealth | null> {
    try {
      const url = `${this.baseUrl}/health`;
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), this.timeout);

      const response = await fetch(url, { signal: controller.signal });
      clearTimeout(timeoutId);

      if (!response.ok) {
        return null;
      }

      return await response.json() as MiniVaultHealth;
    } catch (error) {
      return null;
    }
  }

  isAvailable(): boolean {
    return !!this.baseUrl;
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = { MiniVault };
}
