/**
 * MiniVault HTTP Client for TypeScript/JavaScript
 *
 * High-performance distributed cache client for MiniVault HTTP protocol.
 *
 * Features:
 * - 100,000+ req/sec per worker
 * - 3-node geo-replication
 * - Eventually consistent (30-50ms)
 * - Max value size: 100MB
 * - Zero-cost HTTP abstraction
 *
 * @example
 * ```typescript
 * const vault = new MiniVault('http://localhost:8080', 'your-api-key');
 *
 * // Store JSON
 * await vault.set('user:123', { name: 'Alice', age: 30 });
 *
 * // Retrieve JSON
 * const user = await vault.get<User>('user:123');
 *
 * // Store binary data
 * await vault.setRaw('image:logo', imageBuffer);
 *
 * // Delete
 * await vault.delete('user:123');
 *
 * // Health check
 * const health = await vault.health();
 * ```
 */

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
  /** Base URL of MiniVault server */
  baseUrl: string;
  /** API key for authentication (required for writes if authmode=writes/all) */
  apiKey?: string;
  /** Enable debug logging */
  enableLogging?: boolean;
  /** Request timeout in milliseconds (default: 5000) */
  timeout?: number;
}

export class MiniVault {
  private baseUrl: string;
  private apiKey?: string;
  private enableLogging: boolean;
  private timeout: number;

  constructor(options: MiniVaultOptions | string, apiKey?: string, enableLogging: boolean = false) {
    if (typeof options === 'string') {
      // Legacy constructor: new MiniVault(baseUrl, apiKey, enableLogging)
      this.baseUrl = options;
      this.apiKey = apiKey;
      this.enableLogging = enableLogging;
      this.timeout = 5000;
    } else {
      this.baseUrl = options.baseUrl;
      this.apiKey = options.apiKey;
      this.enableLogging = options.enableLogging || false;
      this.timeout = options.timeout || 5000;
    }
  }

  private log(message: string, ...args: any[]): void {
    if (this.enableLogging) {
      console.log(`[MiniVault] ${message}`, ...args);
    }
  }

  private logError(message: string, error?: any): void {
    console.error(`[MiniVault] ${message}`, error);
  }

  /**
   * Get a value and parse as JSON
   */
  async get<T = any>(key: string): Promise<T | null> {
    try {
      const data = await this.getRaw(key);
      if (!data) return null;

      const text = new TextDecoder().decode(data);
      return JSON.parse(text) as T;
    } catch (error) {
      this.logError(`Failed to parse JSON for key ${key}:`, error);
      return null;
    }
  }

  /**
   * Get raw binary data
   */
  async getRaw(key: string): Promise<Uint8Array | null> {
    try {
      const url = `${this.baseUrl}/${key}`;
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), this.timeout);

      const response = await fetch(url, {
        method: 'GET',
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      if (response.status === 404) {
        this.log(`Cache miss: ${key}`);
        return null;
      }

      if (!response.ok) {
        this.logError(`GET failed for ${key}: ${response.status} ${response.statusText}`);
        return null;
      }

      const data = new Uint8Array(await response.arrayBuffer());
      this.log(`Cache hit: ${key} (${data.length} bytes)`);
      return data;
    } catch (error) {
      if ((error as Error).name === 'AbortError') {
        this.logError(`GET timeout for ${key}`);
      } else {
        this.logError(`GET error for ${key}:`, error);
      }
      return null;
    }
  }

  /**
   * Set a value (automatically serializes to JSON)
   */
  async set(key: string, value: any): Promise<boolean> {
    const json = JSON.stringify(value);
    const data = new TextEncoder().encode(json);
    return this.setRaw(key, data);
  }

  /**
   * Set raw binary data
   */
  async setRaw(key: string, data: Uint8Array | ArrayBuffer): Promise<boolean> {
    try {
      const url = `${this.baseUrl}/${key}`;
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), this.timeout);

      const headers: HeadersInit = {
        'Content-Type': 'application/octet-stream',
      };

      if (this.apiKey) {
        headers['X-API-Key'] = this.apiKey;
      }

      const response = await fetch(url, {
        method: 'PUT',
        headers,
        body: data,
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        const text = await response.text();
        this.logError(`SET failed for ${key}: ${response.status} ${text}`);
        return false;
      }

      this.log(`Cache set: ${key} (${data.byteLength} bytes)`);
      return true;
    } catch (error) {
      if ((error as Error).name === 'AbortError') {
        this.logError(`SET timeout for ${key}`);
      } else {
        this.logError(`SET error for ${key}:`, error);
      }
      return false;
    }
  }

  /**
   * Delete a key
   */
  async delete(key: string): Promise<boolean> {
    try {
      const url = `${this.baseUrl}/${key}`;
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), this.timeout);

      const headers: HeadersInit = {};
      if (this.apiKey) {
        headers['X-API-Key'] = this.apiKey;
      }

      const response = await fetch(url, {
        method: 'DELETE',
        headers,
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        this.logError(`DELETE failed for ${key}: ${response.status}`);
        return false;
      }

      this.log(`Cache delete: ${key}`);
      return true;
    } catch (error) {
      if ((error as Error).name === 'AbortError') {
        this.logError(`DELETE timeout for ${key}`);
      } else {
        this.logError(`DELETE error for ${key}:`, error);
      }
      return false;
    }
  }

  /**
   * Batch get multiple keys (parallelized)
   */
  async mget<T = any>(keys: string[]): Promise<Array<T | null>> {
    const promises = keys.map(key => this.get<T>(key));
    return Promise.all(promises);
  }

  /**
   * Batch set multiple key-value pairs (parallelized)
   */
  async mset(entries: Array<{ key: string; value: any }>): Promise<boolean[]> {
    const promises = entries.map(({ key, value }) => this.set(key, value));
    return Promise.all(promises);
  }

  /**
   * Check if a key exists
   */
  async exists(key: string): Promise<boolean> {
    const data = await this.getRaw(key);
    return data !== null;
  }

  /**
   * Get cluster health
   */
  async health(): Promise<MiniVaultHealth | null> {
    try {
      const url = `${this.baseUrl}/health`;
      const response = await fetch(url);

      if (!response.ok) {
        this.logError(`Health check failed: ${response.status}`);
        return null;
      }

      return await response.json() as MiniVaultHealth;
    } catch (error) {
      this.logError('Health check error:', error);
      return null;
    }
  }

  /**
   * Check if vault is configured and available
   */
  isAvailable(): boolean {
    return !!this.baseUrl;
  }
}

// Export for CommonJS compatibility
if (typeof module !== 'undefined' && module.exports) {
  module.exports = { MiniVault };
}
