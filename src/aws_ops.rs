use anyhow::{Context, Result};
use aws_sdk_s3::types::{GlacierJobParameters, RestoreRequest, Tier};
use std::fs;
use std::path::Path;

use crate::log;

#[derive(Debug, Clone, Default)]
pub struct ObjectInfo {
    pub key: String,
    pub size: i64,
    pub last_modified: String,
    pub storage_class: String,
    pub restore_status: String,
    pub content_type: String,
    pub etag: String,
    pub is_prefix: bool,
}

async fn make_client(region: &str, profile: &str) -> Result<aws_sdk_s3::Client> {
    log::log(&format!(
        "make_client region={:?} profile={:?}",
        region, profile
    ));
    let mut loader = aws_config::defaults(aws_config::BehaviorVersion::latest());
    if !region.is_empty() {
        loader = loader.region(aws_config::Region::new(region.to_string()));
    }
    if !profile.is_empty() {
        loader = loader.profile_name(profile);
    }
    let sdk_config = loader.load().await;
    log::log(&format!(
        "SDK config loaded, region={:?}",
        sdk_config.region()
    ));
    Ok(aws_sdk_s3::Client::new(&sdk_config))
}

fn blocking<F: std::future::Future>(f: F) -> F::Output {
    tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .unwrap()
        .block_on(f)
}

pub fn list_objects(
    bucket: &str,
    prefix: &str,
    region: &str,
    profile: &str,
) -> Result<Vec<ObjectInfo>> {
    log::log(&format!(
        "list_objects bucket={:?} prefix={:?} region={:?} profile={:?}",
        bucket, prefix, region, profile
    ));
    blocking(async {
        let client = make_client(region, profile).await?;
        log::log("list_objects: sending request...");
        let result = client
            .list_objects_v2()
            .bucket(bucket)
            .prefix(prefix)
            .delimiter("/")
            .send()
            .await;
        if let Err(ref e) = result {
            log::log(&format!("list_objects ERROR: {:?}", e));
        }
        let resp = result.context("listing objects")?;

        let mut items = Vec::new();

        // Prefixes (directories)
        for p in resp.common_prefixes() {
            if let Some(pref) = p.prefix() {
                items.push(ObjectInfo {
                    key: pref.to_string(),
                    is_prefix: true,
                    ..Default::default()
                });
            }
        }

        // Objects
        for obj in resp.contents() {
            let key = obj.key().unwrap_or_default();
            // Skip the prefix itself
            if key == prefix {
                continue;
            }
            items.push(ObjectInfo {
                key: key.to_string(),
                size: obj.size().unwrap_or(0),
                last_modified: obj
                    .last_modified()
                    .and_then(|t| t.fmt(aws_sdk_s3::primitives::DateTimeFormat::DateTime).ok())
                    .unwrap_or_default(),
                storage_class: obj
                    .storage_class()
                    .map(|c| c.as_str().to_string())
                    .unwrap_or_else(|| "STANDARD".to_string()),
                ..Default::default()
            });
        }

        Ok(items)
    })
}

pub fn head_object(bucket: &str, key: &str, region: &str, profile: &str) -> Result<ObjectInfo> {
    blocking(async {
        let client = make_client(region, profile).await?;
        let resp = client
            .head_object()
            .bucket(bucket)
            .key(key)
            .send()
            .await
            .context("head object")?;

        let restore_status = resp
            .restore()
            .map(|r| {
                if r.contains("ongoing-request=\"true\"") {
                    "in-progress".to_string()
                } else if r.contains("ongoing-request=\"false\"") {
                    "available".to_string()
                } else {
                    String::new()
                }
            })
            .unwrap_or_default();

        Ok(ObjectInfo {
            key: key.to_string(),
            size: resp.content_length().unwrap_or(0),
            last_modified: resp
                .last_modified()
                .and_then(|t| t.fmt(aws_sdk_s3::primitives::DateTimeFormat::DateTime).ok())
                .unwrap_or_default(),
            storage_class: resp
                .storage_class()
                .map(|c| c.as_str().to_string())
                .unwrap_or_else(|| "STANDARD".to_string()),
            restore_status,
            content_type: resp.content_type().unwrap_or_default().to_string(),
            etag: resp.e_tag().unwrap_or_default().to_string(),
            is_prefix: false,
        })
    })
}

pub fn list_buckets(region: &str, profile: &str) -> Result<Vec<String>> {
    log::log(&format!(
        "list_buckets called region={:?} profile={:?}",
        region, profile
    ));
    blocking(async {
        let client = make_client(region, profile).await?;
        log::log("list_buckets: sending request...");
        let result = client.list_buckets().send().await;
        match &result {
            Ok(resp) => {
                let count = resp.buckets().len();
                log::log(&format!("list_buckets: got {} buckets", count));
            }
            Err(e) => {
                log::log(&format!("list_buckets ERROR: {:?}", e));
            }
        }
        let resp = result.context("listing buckets")?;
        let names: Vec<String> = resp
            .buckets()
            .iter()
            .filter_map(|b| b.name().map(|n| n.to_string()))
            .collect();
        Ok(names)
    })
}

pub fn download(bucket: &str, key: &str, region: &str, profile: &str) -> Result<String> {
    blocking(async {
        let client = make_client(region, profile).await?;
        let resp = client
            .get_object()
            .bucket(bucket)
            .key(key)
            .send()
            .await
            .context("downloading object")?;

        let local_path = Path::new(key)
            .file_name()
            .unwrap_or_default()
            .to_string_lossy()
            .to_string();

        let body = resp.body.collect().await.context("reading body")?;
        fs::write(&local_path, body.into_bytes()).context("writing file")?;

        Ok(local_path)
    })
}

pub fn download_to(
    bucket: &str,
    key: &str,
    region: &str,
    profile: &str,
    dest: &Path,
) -> Result<()> {
    blocking(async {
        let client = make_client(region, profile).await?;
        let resp = client
            .get_object()
            .bucket(bucket)
            .key(key)
            .send()
            .await
            .context("downloading object")?;

        let body = resp.body.collect().await.context("reading body")?;
        fs::write(dest, body.into_bytes()).context("writing file")?;

        Ok(())
    })
}

pub fn restore_object(
    bucket: &str,
    key: &str,
    days: i32,
    tier: &str,
    region: &str,
    profile: &str,
) -> Result<()> {
    blocking(async {
        let client = make_client(region, profile).await?;
        let sdk_tier = match tier {
            "Expedited" => Tier::Expedited,
            "Bulk" => Tier::Bulk,
            _ => Tier::Standard,
        };
        let restore_req = RestoreRequest::builder()
            .days(days)
            .glacier_job_parameters(GlacierJobParameters::builder().tier(sdk_tier).build()?)
            .build();
        client
            .restore_object()
            .bucket(bucket)
            .key(key)
            .restore_request(restore_req)
            .send()
            .await
            .context("restoring object")?;
        Ok(())
    })
}

/// List all objects recursively (no delimiter).
pub fn list_objects_recursive(
    bucket: &str,
    prefix: &str,
    region: &str,
    profile: &str,
) -> Result<Vec<ObjectInfo>> {
    blocking(async {
        let client = make_client(region, profile).await?;
        let mut items = Vec::new();
        let mut continuation: Option<String> = None;

        loop {
            let mut req = client.list_objects_v2().bucket(bucket).prefix(prefix);
            if let Some(token) = &continuation {
                req = req.continuation_token(token);
            }
            let resp = req.send().await.context("listing objects")?;

            for obj in resp.contents() {
                let key = obj.key().unwrap_or_default();
                if key == prefix {
                    continue;
                }
                items.push(ObjectInfo {
                    key: key.to_string(),
                    size: obj.size().unwrap_or(0),
                    last_modified: obj
                        .last_modified()
                        .and_then(|t| t.fmt(aws_sdk_s3::primitives::DateTimeFormat::DateTime).ok())
                        .unwrap_or_default(),
                    storage_class: obj
                        .storage_class()
                        .map(|c| c.as_str().to_string())
                        .unwrap_or_else(|| "STANDARD".to_string()),
                    ..Default::default()
                });
            }

            if resp.is_truncated() == Some(true) {
                continuation = resp.next_continuation_token().map(|s| s.to_string());
            } else {
                break;
            }
        }

        Ok(items)
    })
}

/// Download object and return bytes (for stdout output).
pub fn download_bytes(bucket: &str, key: &str, region: &str, profile: &str) -> Result<Vec<u8>> {
    blocking(async {
        let client = make_client(region, profile).await?;
        let resp = client
            .get_object()
            .bucket(bucket)
            .key(key)
            .send()
            .await
            .context("downloading object")?;
        let body = resp.body.collect().await.context("reading body")?;
        Ok(body.into_bytes().to_vec())
    })
}

/// Download object to a specific file path.
pub fn download_to_path(
    bucket: &str,
    key: &str,
    region: &str,
    profile: &str,
    dest: &Path,
) -> Result<()> {
    blocking(async {
        let client = make_client(region, profile).await?;
        let resp = client
            .get_object()
            .bucket(bucket)
            .key(key)
            .send()
            .await
            .context("downloading object")?;
        let body = resp.body.collect().await.context("reading body")?;
        if let Some(parent) = dest.parent() {
            fs::create_dir_all(parent).ok();
        }
        fs::write(dest, body.into_bytes()).context("writing file")?;
        Ok(())
    })
}

/// Upload a local file to S3.
pub fn upload(
    bucket: &str,
    key: &str,
    local_path: &Path,
    storage_class: &str,
    region: &str,
    profile: &str,
) -> Result<()> {
    blocking(async {
        let client = make_client(region, profile).await?;
        let body = aws_sdk_s3::primitives::ByteStream::from_path(local_path)
            .await
            .context("reading local file")?;

        let sc = storage_class
            .parse::<aws_sdk_s3::types::StorageClass>()
            .unwrap_or(aws_sdk_s3::types::StorageClass::DeepArchive);

        client
            .put_object()
            .bucket(bucket)
            .key(key)
            .storage_class(sc)
            .body(body)
            .send()
            .await
            .context("uploading object")?;
        Ok(())
    })
}

pub fn presign_get(
    bucket: &str,
    key: &str,
    expires_secs: u64,
    region: &str,
    profile: &str,
) -> Result<String> {
    blocking(async {
        let client = make_client(region, profile).await?;
        let presign_config = aws_sdk_s3::presigning::PresigningConfig::expires_in(
            std::time::Duration::from_secs(expires_secs),
        )
        .context("invalid presign duration")?;
        let presigned = client
            .get_object()
            .bucket(bucket)
            .key(key)
            .presigned(presign_config)
            .await
            .context("generating presigned URL")?;
        Ok(presigned.uri().to_string())
    })
}

pub fn test_profile(region: &str, profile: &str) -> Result<()> {
    blocking(async {
        let client = make_client(region, profile).await?;
        client
            .list_buckets()
            .send()
            .await
            .context("testing profile")?;
        Ok(())
    })
}
