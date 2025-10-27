import * as net from 'net';

const OpGet = 0x01;
const OpSet = 0x02;
const OpDelete = 0x03;
const OpHealth = 0x05;
const OpAuth = 0x06;

const StatusSuccess = 0x00;

export interface MiniVaultBinaryOptions {
  address: string;
  apiKey?: string;
  timeout?: number;
  poolSize?: number;
}

export class MiniVaultBinary {
  private address: string;
  private apiKey?: string;
  private timeout: number;
  private host: string;
  private port: number;
  private pool: net.Socket[] = [];
  private poolSize: number;

  constructor(options: MiniVaultBinaryOptions | string, apiKey?: string) {
    if (typeof options === 'string') {
      this.address = options;
      this.apiKey = apiKey;
      this.timeout = 5000;
      this.poolSize = 5;
    } else {
      this.address = options.address;
      this.apiKey = options.apiKey;
      this.timeout = options.timeout || 5000;
      this.poolSize = options.poolSize || 5;
    }

    const [host, port] = this.address.split(':');
    this.host = host;
    this.port = parseInt(port, 10);
  }

  private async getConnection(): Promise<net.Socket> {
    if (this.pool.length > 0) {
      return this.pool.pop()!;
    }
    return new Promise((resolve, reject) => {
      const socket = net.createConnection({ host: this.host, port: this.port });
      const timeoutId = setTimeout(() => {
        socket.destroy();
        reject(new Error('connection timeout'));
      }, this.timeout);

      socket.once('connect', () => {
        clearTimeout(timeoutId);
        resolve(socket);
      });

      socket.once('error', (err) => {
        clearTimeout(timeoutId);
        reject(err);
      });
    });
  }

  private releaseConnection(socket: net.Socket) {
    if (this.pool.length < this.poolSize) {
      socket.removeAllListeners();
      this.pool.push(socket);
    } else {
      socket.destroy();
    }
  }

  private async sendRequest(socket: net.Socket, request: Buffer): Promise<Buffer> {
    return new Promise((resolve, reject) => {
      let responseBuffer = Buffer.alloc(0);
      let headerReceived = false;
      let expectedDataLength = 0;

      const timeoutId = setTimeout(() => {
        socket.destroy();
        reject(new Error('request timeout'));
      }, this.timeout);

      const onData = (chunk: Buffer) => {
        responseBuffer = Buffer.concat([responseBuffer, chunk]);

        if (!headerReceived && responseBuffer.length >= 5) {
          const status = responseBuffer[0];
          expectedDataLength = responseBuffer.readUInt32LE(1);
          headerReceived = true;

          if (status !== StatusSuccess) {
            clearTimeout(timeoutId);
            socket.removeListener('data', onData);
            socket.removeListener('error', onError);
            reject(new Error(`server error: 0x${status.toString(16)}`));
            return;
          }
        }

        if (headerReceived && responseBuffer.length >= 5 + expectedDataLength) {
          clearTimeout(timeoutId);
          socket.removeListener('data', onData);
          socket.removeListener('error', onError);
          const data = responseBuffer.slice(5, 5 + expectedDataLength);
          resolve(data);
        }
      };

      const onError = (err: Error) => {
        clearTimeout(timeoutId);
        socket.removeListener('data', onData);
        socket.removeListener('error', onError);
        reject(err);
      };

      socket.on('data', onData);
      socket.once('error', onError);
      socket.write(request);
    });
  }

  async get(key: string): Promise<Buffer | null> {
    const socket = await this.getConnection();

    try {
      if (this.apiKey) {
        const authReq = Buffer.allocUnsafe(3 + this.apiKey.length);
        authReq[0] = OpAuth;
        authReq.writeUInt16LE(this.apiKey.length, 1);
        authReq.write(this.apiKey, 3);
        await this.sendRequest(socket, authReq);
      }

      const keyBuffer = Buffer.from(key, 'utf8');
      const request = Buffer.allocUnsafe(3 + keyBuffer.length);
      request[0] = OpGet;
      request.writeUInt16LE(keyBuffer.length, 1);
      keyBuffer.copy(request, 3);

      const response = await this.sendRequest(socket, request);
      this.releaseConnection(socket);
      return response.length > 0 ? response : null;
    } catch (err) {
      socket.destroy();
      return null;
    }
  }

  async set(key: string, value: Buffer): Promise<boolean> {
    const socket = await this.getConnection();

    try {
      if (this.apiKey) {
        const authReq = Buffer.allocUnsafe(3 + this.apiKey.length);
        authReq[0] = OpAuth;
        authReq.writeUInt16LE(this.apiKey.length, 1);
        authReq.write(this.apiKey, 3);
        await this.sendRequest(socket, authReq);
      }

      const keyBuffer = Buffer.from(key, 'utf8');
      const request = Buffer.allocUnsafe(3 + keyBuffer.length + 5 + value.length);
      request[0] = OpSet;
      request.writeUInt16LE(keyBuffer.length, 1);
      keyBuffer.copy(request, 3);
      request.writeUInt32LE(value.length, 3 + keyBuffer.length);
      request[3 + keyBuffer.length + 4] = 0;
      value.copy(request, 3 + keyBuffer.length + 5);

      await this.sendRequest(socket, request);
      this.releaseConnection(socket);
      return true;
    } catch (err) {
      socket.destroy();
      return false;
    }
  }

  async delete(key: string): Promise<boolean> {
    const socket = await this.getConnection();

    try {
      if (this.apiKey) {
        const authReq = Buffer.allocUnsafe(3 + this.apiKey.length);
        authReq[0] = OpAuth;
        authReq.writeUInt16LE(this.apiKey.length, 1);
        authReq.write(this.apiKey, 3);
        await this.sendRequest(socket, authReq);
      }

      const keyBuffer = Buffer.from(key, 'utf8');
      const request = Buffer.allocUnsafe(3 + keyBuffer.length);
      request[0] = OpDelete;
      request.writeUInt16LE(keyBuffer.length, 1);
      keyBuffer.copy(request, 3);

      await this.sendRequest(socket, request);
      this.releaseConnection(socket);
      return true;
    } catch (err) {
      socket.destroy();
      return false;
    }
  }

  async health(): Promise<any> {
    const socket = await this.getConnection();

    try {
      const request = Buffer.allocUnsafe(3 + 6);
      request[0] = OpHealth;
      request.writeUInt16LE(6, 1);
      request.write('health', 3);

      const data = await this.sendRequest(socket, request);
      this.releaseConnection(socket);
      return JSON.parse(data.toString('utf8'));
    } catch (err) {
      socket.destroy();
      return null;
    }
  }

  close() {
    this.pool.forEach(s => s.destroy());
    this.pool = [];
  }
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = { MiniVaultBinary };
}
