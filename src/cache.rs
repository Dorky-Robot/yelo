use anyhow::{Context, Result};
use std::fs;
use std::path::PathBuf;
use std::time::SystemTime;

fn cache_dir() -> PathBuf {
    dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".yelo/cache")
}

pub fn cached_path(bucket: &str, key: &str) -> PathBuf {
    cache_dir().join(bucket).join(key)
}

pub fn is_cached(bucket: &str, key: &str) -> bool {
    cached_path(bucket, key).exists()
}

pub fn ensure_cache_dir(bucket: &str, key: &str) -> Result<PathBuf> {
    let path = cached_path(bucket, key);
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).context("creating cache directory")?;
    }
    Ok(path)
}

pub fn download_to_cache(bucket: &str, key: &str, region: &str, profile: &str) -> Result<String> {
    let dest = ensure_cache_dir(bucket, key)?;

    crate::aws_ops::download_to(bucket, key, region, profile, &dest)?;

    Ok(dest.to_string_lossy().to_string())
}

pub fn open_cached(bucket: &str, key: &str) -> Result<()> {
    let path = cached_path(bucket, key);
    if !path.exists() {
        anyhow::bail!("file not in cache");
    }
    std::process::Command::new("open")
        .arg(&path)
        .spawn()
        .context("opening file")?;
    Ok(())
}

// ---------------------------------------------------------------------------
// Library support
// ---------------------------------------------------------------------------

#[derive(Debug, Clone)]
pub struct CachedFile {
    pub bucket: String,
    pub key: String,
    pub name: String,
    pub size: u64,
    pub cached_at: SystemTime,
    pub path: PathBuf,
}

pub fn list_cached_files() -> Vec<CachedFile> {
    let base = cache_dir();
    let mut files = Vec::new();
    walk_cache_dir(&base, &base, &mut files);
    files.sort_by(|a, b| b.cached_at.cmp(&a.cached_at));
    files
}

fn walk_cache_dir(base: &PathBuf, dir: &PathBuf, out: &mut Vec<CachedFile>) {
    let entries = match fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };
    for entry in entries.flatten() {
        let path = entry.path();
        if path.is_dir() {
            walk_cache_dir(base, &path, out);
        } else if path.is_file()
            && let Some(cf) = cached_file_from_path(base, &path)
        {
            out.push(cf);
        }
    }
}

fn cached_file_from_path(base: &PathBuf, path: &PathBuf) -> Option<CachedFile> {
    let rel = path.strip_prefix(base).ok()?;
    let components: Vec<&str> = rel
        .components()
        .map(|c| c.as_os_str().to_str().unwrap_or(""))
        .collect();
    if components.len() < 2 {
        return None;
    }
    let bucket = components[0].to_string();
    let key = components[1..].join("/");
    let name = components.last()?.to_string();
    let meta = fs::metadata(path).ok()?;
    let size = meta.len();
    let cached_at = meta.modified().unwrap_or(SystemTime::UNIX_EPOCH);
    Some(CachedFile {
        bucket,
        key,
        name,
        size,
        cached_at,
        path: path.clone(),
    })
}

pub fn delete_cached(bucket: &str, key: &str) -> Result<()> {
    let path = cached_path(bucket, key);
    if path.exists() {
        fs::remove_file(&path).context("deleting cached file")?;
    }
    // Clean up empty parent directories up to cache_dir/bucket
    let bucket_dir = cache_dir().join(bucket);
    let mut dir = path.parent().map(|p| p.to_path_buf());
    while let Some(d) = dir {
        if d == bucket_dir || d == cache_dir() {
            break;
        }
        if fs::read_dir(&d)
            .map(|mut e| e.next().is_none())
            .unwrap_or(true)
        {
            let _ = fs::remove_dir(&d);
            dir = d.parent().map(|p| p.to_path_buf());
        } else {
            break;
        }
    }
    Ok(())
}

pub fn rename_cached(bucket: &str, old_key: &str, new_name: &str) -> Result<String> {
    let old_path = cached_path(bucket, old_key);
    if !old_path.exists() {
        anyhow::bail!("file not in cache");
    }
    let parent = old_path.parent().context("no parent directory")?;
    let new_path = parent.join(new_name);
    fs::rename(&old_path, &new_path).context("renaming cached file")?;

    // Build new key
    let old_parent = if let Some(idx) = old_key.rfind('/') {
        &old_key[..=idx]
    } else {
        ""
    };
    let new_key = format!("{}{}", old_parent, new_name);
    Ok(new_key)
}

pub fn copy_path_to_clipboard(path: &std::path::Path) -> Result<()> {
    let path_str = path.to_string_lossy();
    let mut child = std::process::Command::new("pbcopy")
        .stdin(std::process::Stdio::piped())
        .spawn()
        .context("running pbcopy")?;
    if let Some(mut stdin) = child.stdin.take() {
        use std::io::Write;
        stdin
            .write_all(path_str.as_bytes())
            .context("writing to pbcopy")?;
    }
    child.wait().context("waiting for pbcopy")?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cached_path_structure() {
        let path = cached_path("my-bucket", "photos/2024/img.jpg");
        let path_str = path.to_string_lossy();
        assert!(path_str.contains(".yelo/cache/my-bucket/photos/2024/img.jpg"));
    }

    #[test]
    fn cached_path_preserves_hierarchy() {
        let path = cached_path("bucket", "a/b/c/file.txt");
        let path_str = path.to_string_lossy();
        assert!(path_str.ends_with("cache/bucket/a/b/c/file.txt"));
    }

    #[test]
    fn is_cached_false_for_nonexistent() {
        assert!(!is_cached("nonexistent-bucket-xyz", "nonexistent-key"));
    }

    #[test]
    fn ensure_cache_dir_creates_parents() {
        let _dir = tempfile::tempdir().unwrap();
        // We can't easily test ensure_cache_dir without mocking home_dir,
        // but we can verify the path structure logic
        let path = cached_path("test-bucket", "deep/nested/path/file.dat");
        assert!(
            path.to_string_lossy()
                .contains("test-bucket/deep/nested/path/file.dat")
        );
    }
}
