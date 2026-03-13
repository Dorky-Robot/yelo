use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct BucketConfig {
    pub name: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub region: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub profile: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct DaemonConfig {
    /// Poll interval in seconds (default: 60)
    #[serde(
        default = "default_poll_interval",
        skip_serializing_if = "is_default_poll"
    )]
    pub poll_interval: u64,
}

fn default_poll_interval() -> u64 {
    60
}

fn is_default_poll(v: &u64) -> bool {
    *v == 0 || *v == 60
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Config {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub default_bucket: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub buckets: Vec<BucketConfig>,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub region: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub profile: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub daemon: Option<DaemonConfig>,
}

impl Config {
    pub fn daemon_poll_interval(&self) -> u64 {
        self.daemon
            .as_ref()
            .map(|d| d.poll_interval)
            .filter(|&v| v > 0)
            .unwrap_or(60)
    }

    pub fn path() -> PathBuf {
        dirs::home_dir()
            .unwrap_or_else(|| PathBuf::from("."))
            .join(".yelo/config.yaml")
    }

    pub fn load() -> Result<Self> {
        let path = Self::path();
        if !path.exists() {
            return Ok(Self::default());
        }
        let data = fs::read_to_string(&path).context("reading config")?;
        let cfg: Config = serde_yaml::from_str(&data).context("parsing config")?;
        Ok(cfg)
    }

    pub fn save(&self) -> Result<()> {
        let path = Self::path();
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent).context("creating config dir")?;
        }
        let data = serde_yaml::to_string(self).context("serializing config")?;
        fs::write(&path, data).context("writing config")?;
        Ok(())
    }

    pub fn add_bucket(&mut self, name: &str, region: &str, profile: &str) {
        if let Some(b) = self.buckets.iter_mut().find(|b| b.name == name) {
            b.region = region.to_string();
            b.profile = profile.to_string();
            return;
        }
        self.buckets.push(BucketConfig {
            name: name.to_string(),
            region: region.to_string(),
            profile: profile.to_string(),
        });
    }

    pub fn remove_bucket(&mut self, name: &str) -> bool {
        let len = self.buckets.len();
        self.buckets.retain(|b| b.name != name);
        if self.default_bucket == name {
            self.default_bucket.clear();
        }
        self.buckets.len() < len
    }

    pub fn set_default(&mut self, name: &str) -> bool {
        if self.buckets.iter().any(|b| b.name == name) {
            self.default_bucket = name.to_string();
            true
        } else {
            false
        }
    }

    pub fn get_bucket(&self, name: &str) -> Option<&BucketConfig> {
        self.buckets.iter().find(|b| b.name == name)
    }

    pub fn resolve_bucket(&self) -> Option<String> {
        if !self.default_bucket.is_empty() {
            return Some(self.default_bucket.clone());
        }
        if self.buckets.len() == 1 {
            return Some(self.buckets[0].name.clone());
        }
        None
    }

    pub fn resolve_region(&self, bucket: &str) -> String {
        if let Some(b) = self.get_bucket(bucket)
            && !b.region.is_empty()
        {
            return b.region.clone();
        }
        self.region.clone()
    }

    pub fn resolve_profile(&self, bucket: &str) -> String {
        if let Some(b) = self.get_bucket(bucket)
            && !b.profile.is_empty()
        {
            return b.profile.clone();
        }
        self.profile.clone()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn cfg_with_buckets() -> Config {
        Config {
            default_bucket: "primary".into(),
            buckets: vec![
                BucketConfig {
                    name: "primary".into(),
                    region: "us-west-2".into(),
                    profile: "prod".into(),
                },
                BucketConfig {
                    name: "backup".into(),
                    region: "".into(),
                    profile: "".into(),
                },
            ],
            region: "us-east-1".into(),
            profile: "default".into(),
            daemon: None,
        }
    }

    #[test]
    fn resolve_bucket_returns_default() {
        let cfg = cfg_with_buckets();
        assert_eq!(cfg.resolve_bucket(), Some("primary".into()));
    }

    #[test]
    fn resolve_bucket_sole_bucket() {
        let cfg = Config {
            buckets: vec![BucketConfig {
                name: "only".into(),
                ..Default::default()
            }],
            ..Default::default()
        };
        assert_eq!(cfg.resolve_bucket(), Some("only".into()));
    }

    #[test]
    fn resolve_bucket_none_when_ambiguous() {
        let cfg = Config {
            buckets: vec![
                BucketConfig {
                    name: "a".into(),
                    ..Default::default()
                },
                BucketConfig {
                    name: "b".into(),
                    ..Default::default()
                },
            ],
            ..Default::default()
        };
        assert_eq!(cfg.resolve_bucket(), None);
    }

    #[test]
    fn resolve_bucket_none_when_empty() {
        let cfg = Config::default();
        assert_eq!(cfg.resolve_bucket(), None);
    }

    #[test]
    fn resolve_region_bucket_override() {
        let cfg = cfg_with_buckets();
        assert_eq!(cfg.resolve_region("primary"), "us-west-2");
    }

    #[test]
    fn resolve_region_falls_back_to_global() {
        let cfg = cfg_with_buckets();
        assert_eq!(cfg.resolve_region("backup"), "us-east-1");
    }

    #[test]
    fn resolve_region_unknown_bucket_uses_global() {
        let cfg = cfg_with_buckets();
        assert_eq!(cfg.resolve_region("nonexistent"), "us-east-1");
    }

    #[test]
    fn resolve_profile_bucket_override() {
        let cfg = cfg_with_buckets();
        assert_eq!(cfg.resolve_profile("primary"), "prod");
    }

    #[test]
    fn resolve_profile_falls_back_to_global() {
        let cfg = cfg_with_buckets();
        assert_eq!(cfg.resolve_profile("backup"), "default");
    }

    #[test]
    fn add_bucket_new() {
        let mut cfg = Config::default();
        cfg.add_bucket("test", "eu-west-1", "dev");
        assert_eq!(cfg.buckets.len(), 1);
        assert_eq!(cfg.buckets[0].name, "test");
        assert_eq!(cfg.buckets[0].region, "eu-west-1");
    }

    #[test]
    fn add_bucket_updates_existing() {
        let mut cfg = cfg_with_buckets();
        cfg.add_bucket("primary", "ap-south-1", "staging");
        assert_eq!(cfg.buckets.len(), 2);
        assert_eq!(cfg.buckets[0].region, "ap-south-1");
        assert_eq!(cfg.buckets[0].profile, "staging");
    }

    #[test]
    fn remove_bucket_exists() {
        let mut cfg = cfg_with_buckets();
        assert!(cfg.remove_bucket("primary"));
        assert_eq!(cfg.buckets.len(), 1);
        assert!(
            cfg.default_bucket.is_empty(),
            "default cleared when removed"
        );
    }

    #[test]
    fn remove_bucket_nonexistent() {
        let mut cfg = cfg_with_buckets();
        assert!(!cfg.remove_bucket("nope"));
        assert_eq!(cfg.buckets.len(), 2);
    }

    #[test]
    fn set_default_valid() {
        let mut cfg = cfg_with_buckets();
        assert!(cfg.set_default("backup"));
        assert_eq!(cfg.default_bucket, "backup");
    }

    #[test]
    fn set_default_invalid() {
        let mut cfg = cfg_with_buckets();
        assert!(!cfg.set_default("nonexistent"));
        assert_eq!(cfg.default_bucket, "primary");
    }

    #[test]
    fn get_bucket_found() {
        let cfg = cfg_with_buckets();
        assert!(cfg.get_bucket("primary").is_some());
    }

    #[test]
    fn get_bucket_not_found() {
        let cfg = cfg_with_buckets();
        assert!(cfg.get_bucket("nope").is_none());
    }

    #[test]
    fn serde_round_trip() {
        let cfg = cfg_with_buckets();
        let yaml = serde_yaml::to_string(&cfg).unwrap();
        let parsed: Config = serde_yaml::from_str(&yaml).unwrap();
        assert_eq!(parsed.default_bucket, "primary");
        assert_eq!(parsed.buckets.len(), 2);
        assert_eq!(parsed.buckets[0].region, "us-west-2");
    }

    #[test]
    fn serde_empty_fields_omitted() {
        let cfg = Config::default();
        let yaml = serde_yaml::to_string(&cfg).unwrap();
        assert!(!yaml.contains("default_bucket"));
        assert!(!yaml.contains("buckets"));
    }
}
