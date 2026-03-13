use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RestoreNotification {
    pub id: String,
    #[serde(rename = "type")]
    pub type_: String,
    pub status: String,
    pub bucket: String,
    pub key: String,
    pub region: String,
    pub profile: String,
    pub tier: String,
    pub days: i32,
    pub storage_class: String,
    pub requested_at: DateTime<Utc>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub completed_at: Option<DateTime<Utc>>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub last_checked_at: Option<DateTime<Utc>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

fn notifications_dir() -> PathBuf {
    dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".yelo/notifications")
}

pub fn save_request(req: &RestoreNotification) -> Result<()> {
    let dir = notifications_dir();
    fs::create_dir_all(&dir).context("creating notifications dir")?;
    let path = dir.join(format!("{}.json", req.id));
    let data = serde_json::to_string_pretty(req).context("serializing restore request")?;
    fs::write(&path, data).context("writing restore notification")?;
    Ok(())
}

pub fn list_requests() -> Vec<RestoreNotification> {
    let dir = notifications_dir();
    let entries = match fs::read_dir(&dir) {
        Ok(e) => e,
        Err(_) => return Vec::new(),
    };
    let mut requests: Vec<RestoreNotification> = entries
        .filter_map(|entry| {
            let entry = entry.ok()?;
            let path = entry.path();
            if path.extension().and_then(|e| e.to_str()) != Some("json") {
                return None;
            }
            let data = fs::read_to_string(&path).ok()?;
            serde_json::from_str(&data).ok()
        })
        .collect();
    requests.sort_by(|a, b| b.requested_at.cmp(&a.requested_at));
    requests
}

pub fn update_status(id: &str, status: &str, error: Option<&str>) -> Result<()> {
    let path = notifications_dir().join(format!("{}.json", id));
    let data = fs::read_to_string(&path).context("reading restore notification")?;
    let mut req: RestoreNotification =
        serde_json::from_str(&data).context("parsing restore notification")?;
    req.status = status.to_string();
    req.last_checked_at = Some(Utc::now());
    if status == "available" || status == "failed" {
        req.completed_at = Some(Utc::now());
    }
    if let Some(e) = error {
        req.error = Some(e.to_string());
    }
    let data = serde_json::to_string_pretty(&req).context("serializing restore notification")?;
    fs::write(&path, data).context("writing restore notification")?;
    Ok(())
}

pub fn remove_request(id: &str) -> Result<()> {
    let path = notifications_dir().join(format!("{}.json", id));
    fs::remove_file(&path).context("removing restore notification")?;
    Ok(())
}

pub fn submit_restore(
    bucket: &str,
    key: &str,
    region: &str,
    profile: &str,
    days: i32,
    tier: &str,
    storage_class: &str,
) -> Result<RestoreNotification> {
    crate::aws_ops::restore_object(bucket, key, days, tier, region, profile)?;

    let id = format!("restore-{}", &uuid::Uuid::new_v4().to_string()[..6]);
    let req = RestoreNotification {
        id,
        type_: "glacier_restore".to_string(),
        status: "pending".to_string(),
        bucket: bucket.to_string(),
        key: key.to_string(),
        region: region.to_string(),
        profile: profile.to_string(),
        tier: tier.to_string(),
        days,
        storage_class: storage_class.to_string(),
        requested_at: Utc::now(),
        completed_at: None,
        last_checked_at: None,
        error: None,
    };
    save_request(&req)?;
    Ok(req)
}

pub fn check_restore(req: &RestoreNotification) -> Result<String> {
    let info = crate::aws_ops::head_object(&req.bucket, &req.key, &req.region, &req.profile)?;
    let new_status = match info.restore_status.as_str() {
        "available" => "available",
        "in-progress" => "pending",
        _ => "pending",
    };
    Ok(new_status.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn serde_round_trip() {
        let req = RestoreNotification {
            id: "restore-abc123".to_string(),
            type_: "glacier_restore".to_string(),
            status: "pending".to_string(),
            bucket: "my-bucket".to_string(),
            key: "photos/img.jpg".to_string(),
            region: "us-east-1".to_string(),
            profile: "default".to_string(),
            tier: "Standard".to_string(),
            days: 7,
            storage_class: "DEEP_ARCHIVE".to_string(),
            requested_at: Utc::now(),
            completed_at: None,
            last_checked_at: None,
            error: None,
        };
        let json = serde_json::to_string_pretty(&req).unwrap();
        let parsed: RestoreNotification = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.id, "restore-abc123");
        assert_eq!(parsed.status, "pending");
        assert!(parsed.completed_at.is_none());
        assert!(parsed.error.is_none());
    }

    #[test]
    fn serde_with_completed() {
        let req = RestoreNotification {
            id: "restore-def456".to_string(),
            type_: "glacier_restore".to_string(),
            status: "available".to_string(),
            bucket: "my-bucket".to_string(),
            key: "archive/data.tar".to_string(),
            region: "us-west-2".to_string(),
            profile: "prod".to_string(),
            tier: "Bulk".to_string(),
            days: 3,
            storage_class: "GLACIER".to_string(),
            requested_at: Utc::now(),
            completed_at: Some(Utc::now()),
            last_checked_at: Some(Utc::now()),
            error: None,
        };
        let json = serde_json::to_string(&req).unwrap();
        let parsed: RestoreNotification = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.status, "available");
        assert!(parsed.completed_at.is_some());
    }

    #[test]
    fn serde_with_error() {
        let req = RestoreNotification {
            id: "restore-err789".to_string(),
            type_: "glacier_restore".to_string(),
            status: "failed".to_string(),
            bucket: "my-bucket".to_string(),
            key: "broken.bin".to_string(),
            region: "eu-west-1".to_string(),
            profile: "dev".to_string(),
            tier: "Expedited".to_string(),
            days: 1,
            storage_class: "DEEP_ARCHIVE".to_string(),
            requested_at: Utc::now(),
            completed_at: Some(Utc::now()),
            last_checked_at: None,
            error: Some("Expedited not available for DEEP_ARCHIVE".to_string()),
        };
        let json = serde_json::to_string(&req).unwrap();
        let parsed: RestoreNotification = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.status, "failed");
        assert_eq!(
            parsed.error.as_deref(),
            Some("Expedited not available for DEEP_ARCHIVE")
        );
    }

    #[test]
    fn file_io_round_trip() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("test-restore.json");

        let req = RestoreNotification {
            id: "restore-io123".to_string(),
            type_: "glacier_restore".to_string(),
            status: "pending".to_string(),
            bucket: "test-bucket".to_string(),
            key: "test/key.dat".to_string(),
            region: "us-east-1".to_string(),
            profile: "default".to_string(),
            tier: "Standard".to_string(),
            days: 7,
            storage_class: "DEEP_ARCHIVE".to_string(),
            requested_at: Utc::now(),
            completed_at: None,
            last_checked_at: None,
            error: None,
        };

        let data = serde_json::to_string_pretty(&req).unwrap();
        std::fs::write(&path, &data).unwrap();

        let read_data = std::fs::read_to_string(&path).unwrap();
        let parsed: RestoreNotification = serde_json::from_str(&read_data).unwrap();
        assert_eq!(parsed.id, req.id);
        assert_eq!(parsed.bucket, req.bucket);
        assert_eq!(parsed.key, req.key);
    }
}
