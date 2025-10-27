# MiniVault Examples

Client implementations and usage examples for MiniVault in multiple languages. Each language has two clients:
- `http.*` - HTTP protocol client (works from anywhere)
- `binary.*` - Binary TCP protocol client (higher performance)

## TypeScript/JavaScript

**HTTP Client** (`typescript/http.ts`)
```typescript
import { MiniVault } from './examples/typescript/http';

const vault = new MiniVault('http://localhost:8080', 'your-api-key');

// Store and retrieve JSON (automatic wrapping/unwrapping)
await vault.set('user:123', { name: 'Alice', age: 30 });
const user = await vault.get<User>('user:123');
await vault.delete('user:123');

// Store strings, numbers, arrays, objects etc
await vault.set('counter', 42);
await vault.set('message', 'hello world');
await vault.set('items', ['a', 'b', 'c']);

// Health check
const health = await vault.health();
console.log(health.cache_items, health.memory_mb);
```

**Binary Client** (`typescript/binary.ts`)
```typescript
import { MiniVaultBinary } from './examples/typescript/binary';

const client = new MiniVaultBinary('localhost:3000', 'your-api-key');

// Raw operations
await client.set('mykey', Buffer.from('hello world'));
const data = await client.get('mykey');

// JSON (manual serialization)
const user = { name: 'Alice', age: 30 };
await client.set('user:123', Buffer.from(JSON.stringify(user)));
const userData = await client.get('user:123');
const userObj = JSON.parse(userData.toString());

const health = await client.health();
```

## Go

**HTTP Client** (`go/http.go`)
```go
import minivault "github.com/minivault/examples/go"

client := minivault.NewHTTPClient("http://localhost:8080", "your-api-key")

// Store and retrieve any JSON-serializable data
user := User{Name: "Alice", Age: 30}
err := client.SetJSON("user:123", user)

var result User
err := client.GetJSON("user:123", &result)

err := client.Delete("user:123")

// Store strings, numbers, maps, slices etc
err := client.Set("counter", 42)
err := client.Set("message", "hello world")

// Health check
health, err := client.Health()
fmt.Printf("Cache: %d items, %d MB\n", health.CacheItems, health.CacheSizeMB)
```

**Binary Client** (`go/binary.go`)
```go
import minivault "github.com/minivault/examples/go"

client := minivault.NewBinaryClient("localhost:3000", "your-api-key")

// Raw operations
err := client.Set("mykey", []byte("hello world"))
data, err := client.Get("mykey")
err := client.Delete("mykey")

// JSON operations
user := User{Name: "Alice", Age: 30}
err := client.SetJSON("user:123", user)

var result User
err := client.GetJSON("user:123", &result)

health, err := client.Health()
```

## Python

**HTTP Client** (`python/minivault_http.py`)
```python
from minivault_http import MiniVault

vault = MiniVault('http://localhost:8080', 'your-api-key')

# Store and retrieve any JSON-serializable data
vault.set('user:123', {'name': 'Alice', 'age': 30})
user = vault.get('user:123')
vault.delete('user:123')

# Store strings, numbers, lists, dicts etc
vault.set('counter', 42)
vault.set('message', 'hello world')
vault.set('items', ['a', 'b', 'c'])

# Health check
health = vault.health()
print(f"Cache: {health['cache_items']} items")

# Batch operations
results = vault.mget(['key1', 'key2', 'key3'])
vault.mset({'key1': 'val1', 'key2': 'val2'})
```

**Binary Client** (`python/minivault_binary.py`)
```python
from minivault_binary import MiniVaultBinary

client = MiniVaultBinary('localhost:3000', 'your-api-key')

# Raw operations
client.set('mykey', b'hello world')
data = client.get('mykey')
client.delete('mykey')

# JSON operations
client.set_json('user:123', {'name': 'Alice', 'age': 30})
user = client.get_json('user:123')

health = client.health()
```

## Rust

**HTTP Client** (`rust/http.rs`)
```rust
use minivault::MiniVault;

let vault = MiniVault::new(
    "http://localhost:8080".to_string(),
    Some("your-api-key".to_string()),
);

// Store and retrieve JSON-serializable data
#[derive(Serialize, Deserialize)]
struct User {
    name: String,
    age: u32,
}

let user = User { name: "Alice".to_string(), age: 30 };
vault.set_json("user:123", &user).await?;

let result: User = vault.get_json("user:123").await?.unwrap();
vault.delete("user:123").await?;

// Note: Rust HTTP client still uses Vec<u8> for raw data
// (Other languages support storing any JSON-serializable type directly)

// Health check
let health = vault.health().await?;
println!("Cache: {} items", health.cache_items);
```

**Binary Client** (`rust/binary.rs`)
```rust
use minivault::MiniVaultBinary;

let client = MiniVaultBinary::new(
    "localhost:3000".to_string(),
    Some("your-api-key".to_string()),
);

// Raw operations
client.set("mykey", b"hello world")?;
let data = client.get("mykey")?;
client.delete("mykey")?;

// JSON operations
#[derive(Serialize, Deserialize)]
struct User {
    name: String,
    age: u32,
}

let user = User { name: "Alice".to_string(), age: 30 };
client.set_json("user:123", &user)?;

let result: User = client.get_json("user:123")?.unwrap();

let health = client.health()?;
```

## Performance Comparison

**HTTP Protocol:**
- Universal compatibility (browsers, Vercel, any HTTP client)
- ~100k ops/sec per worker
- Direct storage access (zero HTTP→binary translation overhead)
- Best for: web apps, serverless functions, cross-platform access

**Binary Protocol:**
- Maximum performance: 334k writes/sec, 393k reads/sec
- Native TCP protocol (zero HTTP overhead)
- Lower latency
- Best for: backend services, high-throughput systems, microservices

## Authentication

Set `-authmode` flag when starting MiniVault:
- `none` - No authentication (default)
- `writes` - Require auth for SET/DELETE, reads are public
- `all` - Require auth for all operations

Clients automatically handle authentication when API key is provided.
