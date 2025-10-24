// MiniVault Binary Protocol Client for Rust

use serde::{Deserialize, Serialize};
use std::io::{Read, Write};
use std::net::TcpStream;
use std::time::Duration;

const OP_GET: u8 = 0x01;
const OP_SET: u8 = 0x02;
const OP_DELETE: u8 = 0x03;
const OP_HEALTH: u8 = 0x05;
const OP_AUTH: u8 = 0x06;

const STATUS_SUCCESS: u8 = 0x00;

#[derive(Debug, Deserialize)]
pub struct Health {
    pub status: String,
    pub uptime_seconds: i64,
    pub cache_items: i64,
    pub cache_size_mb: i64,
    pub storage_size_mb: i64,
    pub goroutines: i32,
    pub memory_mb: i64,
}

pub struct MiniVaultBinary {
    address: String,
    api_key: Option<String>,
    timeout: Duration,
}

impl MiniVaultBinary {
    pub fn new(address: String, api_key: Option<String>) -> Self {
        Self {
            address,
            api_key,
            timeout: Duration::from_secs(5),
        }
    }

    fn connect(&self) -> Result<TcpStream, Box<dyn std::error::Error>> {
        let stream = TcpStream::connect(&self.address)?;
        stream.set_read_timeout(Some(self.timeout))?;
        stream.set_write_timeout(Some(self.timeout))?;
        Ok(stream)
    }

    fn send_request(
        &self,
        stream: &mut TcpStream,
        request: &[u8],
    ) -> Result<Vec<u8>, Box<dyn std::error::Error>> {
        stream.write_all(request)?;

        let mut header = [0u8; 5];
        stream.read_exact(&mut header)?;

        let status = header[0];
        let data_len = u32::from_le_bytes([header[1], header[2], header[3], header[4]]) as usize;

        if status != STATUS_SUCCESS {
            return Err(format!("Server error: status=0x{:02x}", status).into());
        }

        let mut data = vec![0u8; data_len];
        if data_len > 0 {
            stream.read_exact(&mut data)?;
        }

        Ok(data)
    }

    fn authenticate(&self, stream: &mut TcpStream) -> Result<(), Box<dyn std::error::Error>> {
        if let Some(api_key) = &self.api_key {
            let key_bytes = api_key.as_bytes();
            let mut request = Vec::with_capacity(3 + key_bytes.len());
            request.push(OP_AUTH);
            request.extend_from_slice(&(key_bytes.len() as u16).to_le_bytes());
            request.extend_from_slice(key_bytes);

            self.send_request(stream, &request)?;
        }
        Ok(())
    }

    fn execute_operation(
        &self,
        op: u8,
        key: &str,
        value: Option<&[u8]>,
    ) -> Result<Vec<u8>, Box<dyn std::error::Error>> {
        let mut stream = self.connect()?;
        self.authenticate(&mut stream)?;

        let key_bytes = key.as_bytes();
        let mut request = Vec::new();

        match op {
            OP_GET | OP_DELETE | OP_HEALTH => {
                request.push(op);
                request.extend_from_slice(&(key_bytes.len() as u16).to_le_bytes());
                request.extend_from_slice(key_bytes);
            }
            OP_SET => {
                let value = value.ok_or("Value required for SET")?;
                request.push(op);
                request.extend_from_slice(&(key_bytes.len() as u16).to_le_bytes());
                request.extend_from_slice(key_bytes);
                request.extend_from_slice(&(value.len() as u32).to_le_bytes());
                request.push(0); // not compressed
                request.extend_from_slice(value);
            }
            _ => return Err("Invalid operation".into()),
        }

        self.send_request(&mut stream, &request)
    }

    pub fn get(&self, key: &str) -> Result<Option<Vec<u8>>, Box<dyn std::error::Error>> {
        let data = self.execute_operation(OP_GET, key, None)?;
        Ok(if data.is_empty() { None } else { Some(data) })
    }

    pub fn get_json<T: for<'de> Deserialize<'de>>(
        &self,
        key: &str,
    ) -> Result<Option<T>, Box<dyn std::error::Error>> {
        match self.get(key)? {
            Some(data) => Ok(Some(serde_json::from_slice(&data)?)),
            None => Ok(None),
        }
    }

    pub fn set(&self, key: &str, value: &[u8]) -> Result<(), Box<dyn std::error::Error>> {
        self.execute_operation(OP_SET, key, Some(value))?;
        Ok(())
    }

    pub fn set_json<T: Serialize>(
        &self,
        key: &str,
        value: &T,
    ) -> Result<(), Box<dyn std::error::Error>> {
        let data = serde_json::to_vec(value)?;
        self.set(key, &data)
    }

    pub fn delete(&self, key: &str) -> Result<(), Box<dyn std::error::Error>> {
        self.execute_operation(OP_DELETE, key, None)?;
        Ok(())
    }

    pub fn health(&self) -> Result<Health, Box<dyn std::error::Error>> {
        let data = self.execute_operation(OP_HEALTH, "health", None)?;
        Ok(serde_json::from_slice(&data)?)
    }

    pub fn exists(&self, key: &str) -> Result<bool, Box<dyn std::error::Error>> {
        Ok(self.get(key)?.is_some())
    }
}
