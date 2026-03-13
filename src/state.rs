use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct State {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub bucket: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub prefix: String,
}

impl State {
    pub fn path() -> PathBuf {
        dirs::home_dir()
            .unwrap_or_else(|| PathBuf::from("."))
            .join(".yelo/state.json")
    }

    pub fn load() -> Result<Self> {
        let path = Self::path();
        if !path.exists() {
            return Ok(Self::default());
        }
        let data = fs::read_to_string(&path).context("reading state")?;
        let st: State = serde_json::from_str(&data).context("parsing state")?;
        Ok(st)
    }

    pub fn save(&self) -> Result<()> {
        let path = Self::path();
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent).context("creating state dir")?;
        }
        let data = serde_json::to_string_pretty(self).context("serializing state")?;
        fs::write(&path, data).context("writing state")?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_state_empty() {
        let st = State::default();
        assert!(st.bucket.is_empty());
        assert!(st.prefix.is_empty());
    }

    #[test]
    fn serde_round_trip() {
        let st = State {
            bucket: "my-bucket".into(),
            prefix: "photos/2024/".into(),
        };
        let json = serde_json::to_string_pretty(&st).unwrap();
        let parsed: State = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.bucket, "my-bucket");
        assert_eq!(parsed.prefix, "photos/2024/");
    }

    #[test]
    fn serde_empty_fields_omitted() {
        let st = State::default();
        let json = serde_json::to_string(&st).unwrap();
        assert!(!json.contains("bucket"));
        assert!(!json.contains("prefix"));
    }

    #[test]
    fn serde_partial_fields() {
        let json = r#"{"bucket": "test"}"#;
        let st: State = serde_json::from_str(json).unwrap();
        assert_eq!(st.bucket, "test");
        assert!(st.prefix.is_empty());
    }
}
