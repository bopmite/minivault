// MiniVault HTTP Client for Rust

use reqwest::{Client, StatusCode};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::time::Duration;

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

pub struct MiniVault {
    base_url: String,
    api_key: Option<String>,
    client: Client,
}

impl MiniVault {
    pub fn new(base_url: String, api_key: Option<String>) -> Self {
        let client = Client::builder()
            .timeout(Duration::from_secs(5))
            .build()
            .unwrap();

        Self {
            base_url: base_url.trim_end_matches('/').to_string(),
            api_key,
            client,
        }
    }

    pub async fn get(&self, key: &str) -> Result<Option<Vec<u8>>, Box<dyn std::error::Error>> {
        let url = format!("{}/{}", self.base_url, key);
        let response = self.client.get(&url).send().await?;

        match response.status() {
            StatusCode::OK => Ok(Some(response.bytes().await?.to_vec())),
            StatusCode::NOT_FOUND => Ok(None),
            status => Err(format!("GET failed: {}", status).into()),
        }
    }

    pub async fn get_json<T: for<'de> Deserialize<'de>>(
        &self,
        key: &str,
    ) -> Result<Option<T>, Box<dyn std::error::Error>> {
        match self.get(key).await? {
            Some(data) => Ok(Some(serde_json::from_slice(&data)?)),
            None => Ok(None),
        }
    }

    pub async fn set(&self, key: &str, data: Vec<u8>) -> Result<(), Box<dyn std::error::Error>> {
        let url = format!("{}/{}", self.base_url, key);
        let mut request = self.client.put(&url).body(data);

        if let Some(api_key) = &self.api_key {
            request = request.header("X-API-Key", api_key);
        }

        let response = request.send().await?;

        if !response.status().is_success() {
            return Err(format!("SET failed: {}", response.status()).into());
        }

        Ok(())
    }

    pub async fn set_json<T: Serialize>(
        &self,
        key: &str,
        value: &T,
    ) -> Result<(), Box<dyn std::error::Error>> {
        let data = serde_json::to_vec(value)?;
        self.set(key, data).await
    }

    pub async fn delete(&self, key: &str) -> Result<(), Box<dyn std::error::Error>> {
        let url = format!("{}/{}", self.base_url, key);
        let mut request = self.client.delete(&url);

        if let Some(api_key) = &self.api_key {
            request = request.header("X-API-Key", api_key);
        }

        let response = request.send().await?;

        if !response.status().is_success() {
            return Err(format!("DELETE failed: {}", response.status()).into());
        }

        Ok(())
    }

    pub async fn exists(&self, key: &str) -> Result<bool, Box<dyn std::error::Error>> {
        Ok(self.get(key).await?.is_some())
    }

    pub async fn health(&self) -> Result<Health, Box<dyn std::error::Error>> {
        let url = format!("{}/health", self.base_url);
        let response = self.client.get(&url).send().await?;

        if !response.status().is_success() {
            return Err(format!("Health check failed: {}", response.status()).into());
        }

        Ok(response.json().await?)
    }

    pub async fn mget(&self, keys: &[&str]) -> Result<HashMap<String, Vec<u8>>, Box<dyn std::error::Error>> {
        let mut results = HashMap::new();
        let futures: Vec<_> = keys.iter().map(|key| self.get(key)).collect();

        for (i, result) in futures::future::join_all(futures).await.into_iter().enumerate() {
            if let Ok(Some(data)) = result {
                results.insert(keys[i].to_string(), data);
            }
        }

        Ok(results)
    }

    pub async fn mset(&self, entries: &HashMap<&str, Vec<u8>>) -> Result<(), Box<dyn std::error::Error>> {
        let futures: Vec<_> = entries
            .iter()
            .map(|(key, value)| self.set(key, value.clone()))
            .collect();

        for result in futures::future::join_all(futures).await {
            result?;
        }

        Ok(())
    }
}
