//! Shared pure helper functions used by both CLI and TUI.

/// Check if a storage class requires Glacier restore before download.
pub fn is_glacier(class: &str) -> bool {
    matches!(class, "GLACIER" | "DEEP_ARCHIVE" | "GLACIER_IR")
}

/// Check if a key looks like an image file.
pub fn is_image(key: &str) -> bool {
    const EXTS: &[&str] = &[".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tiff", ".tif"];
    let lower = key.to_lowercase();
    EXTS.iter().any(|ext| lower.ends_with(ext))
}

/// Display name: strip the prefix to show just the relative part.
pub fn display_name<'a>(key: &'a str, prefix: &str) -> &'a str {
    let name = key.strip_prefix(prefix).unwrap_or(key);
    if name.is_empty() { key } else { name }
}

/// Parse `bucket:path` syntax. Returns (bucket, path) where either may be empty.
pub fn parse_bucket_path(input: &str) -> (&str, &str) {
    match input.find(':') {
        Some(idx) => (&input[..idx], &input[idx + 1..]),
        None => ("", input),
    }
}

/// Navigate to parent prefix (strip last path component).
pub fn parent_prefix(prefix: &str) -> String {
    let trimmed = prefix.trim_end_matches('/');
    if trimmed.is_empty() {
        return String::new();
    }
    match trimmed.rfind('/') {
        Some(idx) => format!("{}/", &trimmed[..idx]),
        None => String::new(),
    }
}

/// FTP-style path resolution. Resolves `target` relative to `current`.
/// Returns a clean path without leading `/`.
pub fn resolve_path(current: &str, target: &str) -> String {
    let raw = if target.starts_with('/') {
        target.to_string()
    } else {
        format!("{}/{}", current.trim_end_matches('/'), target)
    };

    // Split and resolve . and ..
    let mut parts: Vec<&str> = Vec::new();
    for component in raw.split('/') {
        match component {
            "" | "." => {}
            ".." => {
                parts.pop();
            }
            other => parts.push(other),
        }
    }

    parts.join("/")
}

/// Like resolve_path, but ensures the result ends with `/` (for prefix queries).
/// Returns empty string if the result would be just "/".
pub fn resolve_prefix(current: &str, target: &str) -> String {
    let resolved = resolve_path(current, target);
    if resolved.is_empty() {
        String::new()
    } else if resolved.ends_with('/') {
        resolved
    } else {
        format!("{}/", resolved)
    }
}

/// Validate that a tier is allowed for the given storage class.
pub fn validate_tier(storage_class: &str, tier: &str) -> Result<(), String> {
    if storage_class == "DEEP_ARCHIVE" && tier == "Expedited" {
        return Err("Expedited retrieval is not available for DEEP_ARCHIVE".into());
    }
    match tier {
        "Standard" | "Bulk" | "Expedited" => Ok(()),
        _ => Err(format!(
            "unknown tier '{}' (Standard, Bulk, Expedited)",
            tier
        )),
    }
}

/// Format byte size for human display.
pub fn format_size(bytes: i64) -> String {
    const KB: f64 = 1024.0;
    const MB: f64 = KB * 1024.0;
    const GB: f64 = MB * 1024.0;
    const TB: f64 = GB * 1024.0;

    let b = bytes as f64;
    if b >= TB {
        format!("{:.1} TB", b / TB)
    } else if b >= GB {
        format!("{:.1} GB", b / GB)
    } else if b >= MB {
        format!("{:.1} MB", b / MB)
    } else if b >= KB {
        format!("{:.1} KB", b / KB)
    } else {
        format!("{} B", bytes)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_glacier() {
        assert!(is_glacier("GLACIER"));
        assert!(is_glacier("DEEP_ARCHIVE"));
        assert!(is_glacier("GLACIER_IR"));
        assert!(!is_glacier("STANDARD"));
        assert!(!is_glacier("STANDARD_IA"));
    }

    #[test]
    fn test_parse_bucket_path() {
        assert_eq!(
            parse_bucket_path("mybucket:path/to"),
            ("mybucket", "path/to")
        );
        assert_eq!(parse_bucket_path("mybucket:"), ("mybucket", ""));
        assert_eq!(parse_bucket_path("just/a/path"), ("", "just/a/path"));
    }

    #[test]
    fn test_resolve_path() {
        assert_eq!(resolve_path("a/b/", "/c"), "c");
        assert_eq!(resolve_path("a/b/", "c"), "a/b/c");
        assert_eq!(resolve_path("a/b/c/", "../d"), "a/b/d");
        assert_eq!(resolve_path("a/b/", "."), "a/b");
        assert_eq!(resolve_path("", "/x/y"), "x/y");
        assert_eq!(resolve_path("a/b/", "../../"), "");
    }

    #[test]
    fn test_resolve_prefix() {
        assert_eq!(resolve_prefix("a/b/", "c"), "a/b/c/");
        assert_eq!(resolve_prefix("a/b/", "/c"), "c/");
        assert_eq!(resolve_prefix("a/b/", "../.."), "");
    }

    #[test]
    fn test_parent_prefix() {
        assert_eq!(parent_prefix("a/b/c/"), "a/b/");
        assert_eq!(parent_prefix("a/"), "");
        assert_eq!(parent_prefix(""), "");
    }

    #[test]
    fn test_validate_tier() {
        assert!(validate_tier("DEEP_ARCHIVE", "Standard").is_ok());
        assert!(validate_tier("DEEP_ARCHIVE", "Bulk").is_ok());
        assert!(validate_tier("DEEP_ARCHIVE", "Expedited").is_err());
        assert!(validate_tier("GLACIER", "Expedited").is_ok());
        assert!(validate_tier("GLACIER", "Nonsense").is_err());
    }

    #[test]
    fn test_format_size() {
        assert_eq!(format_size(0), "0 B");
        assert_eq!(format_size(512), "512 B");
        assert_eq!(format_size(1024), "1.0 KB");
        assert_eq!(format_size(1_048_576), "1.0 MB");
    }
}
