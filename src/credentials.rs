use anyhow::{Context, Result};
use std::fs;
use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};

fn aws_dir() -> PathBuf {
    dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".aws")
}

/// Read all profile names from ~/.aws/credentials and ~/.aws/config.
pub fn read_profiles() -> Result<Vec<String>> {
    let dir = aws_dir();
    let mut seen = std::collections::HashSet::new();
    let mut profiles = Vec::new();

    for rel in &["credentials", "config"] {
        let path = dir.join(rel);
        let f = match fs::File::open(&path) {
            Ok(f) => f,
            Err(_) => continue,
        };
        let reader = BufReader::new(f);
        for line in reader.lines() {
            let line = match line {
                Ok(l) => l,
                Err(_) => continue,
            };
            let trimmed = line.trim();
            if trimmed.starts_with('[') && trimmed.ends_with(']') {
                let name = trimmed[1..trimmed.len() - 1]
                    .strip_prefix("profile ")
                    .unwrap_or(&trimmed[1..trimmed.len() - 1]);
                if seen.insert(name.to_string()) {
                    profiles.push(name.to_string());
                }
            }
        }
    }
    Ok(profiles)
}

/// Read a single value from an INI file section.
pub fn read_ini_value(path: &Path, section: &str, key: &str) -> Option<String> {
    let f = fs::File::open(path).ok()?;
    let reader = BufReader::new(f);
    let mut in_section = false;
    for line in reader.lines() {
        let line = line.ok()?;
        let trimmed = line.trim();
        if trimmed.starts_with('[') && trimmed.ends_with(']') {
            let name = &trimmed[1..trimmed.len() - 1];
            in_section = name == section;
            continue;
        }
        if in_section
            && let Some((k, v)) = trimmed.split_once('=')
            && k.trim() == key
        {
            return Some(v.trim().to_string());
        }
    }
    None
}

/// Merge key=value pairs into an INI section, preserving other keys.
pub fn update_ini_section(path: &Path, section: &str, values: &[(&str, &str)]) -> Result<()> {
    let content = fs::read_to_string(path).unwrap_or_default();
    let lines: Vec<&str> = content.lines().collect();

    // Find section boundaries
    let mut start: Option<usize> = None;
    let mut end: Option<usize> = None;
    for (i, line) in lines.iter().enumerate() {
        let trimmed = line.trim();
        if trimmed == format!("[{}]", section) {
            start = Some(i);
            continue;
        }
        if start.is_some() && end.is_none() && trimmed.starts_with('[') {
            end = Some(i);
        }
    }

    let mut result_lines: Vec<String> = Vec::new();

    if let Some(s) = start {
        let e = end.unwrap_or(lines.len());
        // Trim trailing blank lines from section
        let mut actual_end = e;
        while actual_end > s + 1 && lines[actual_end - 1].trim().is_empty() {
            actual_end -= 1;
        }

        // Read existing key-value pairs in section
        let mut existing: Vec<(String, String)> = Vec::new();
        for line in &lines[s + 1..actual_end] {
            if let Some((k, v)) = line.split_once('=') {
                existing.push((k.trim().to_string(), v.trim().to_string()));
            }
        }

        // Apply updates
        let values_map: std::collections::HashMap<&str, &str> = values.iter().copied().collect();
        for entry in &mut existing {
            if let Some(v) = values_map.get(entry.0.as_str()) {
                entry.1 = v.to_string();
            }
        }
        // Add new keys
        let existing_keys: std::collections::HashSet<String> =
            existing.iter().map(|(k, _)| k.clone()).collect();
        for (k, v) in values {
            if !existing_keys.contains(*k) {
                existing.push((k.to_string(), v.to_string()));
            }
        }

        // Rebuild
        result_lines.extend(lines[..s].iter().map(|s| s.to_string()));
        result_lines.push(format!("[{}]", section));
        for (k, v) in &existing {
            result_lines.push(format!("{} = {}", k, v));
        }
        result_lines.extend(lines[e..].iter().map(|s| s.to_string()));
    } else {
        // Append new section
        result_lines.extend(lines.iter().map(|s| s.to_string()));
        if !result_lines.is_empty() && !result_lines.last().unwrap().trim().is_empty() {
            result_lines.push(String::new());
        }
        result_lines.push(format!("[{}]", section));
        for (k, v) in values {
            result_lines.push(format!("{} = {}", k, v));
        }
        result_lines.push(String::new());
    }

    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).context("creating aws dir")?;
    }
    fs::write(path, result_lines.join("\n")).context("writing INI file")?;
    Ok(())
}

/// Remove a section from an INI file.
pub fn remove_ini_section(path: &Path, section: &str) -> Result<()> {
    let content = fs::read_to_string(path)?;
    let lines: Vec<&str> = content.lines().collect();

    let mut start: Option<usize> = None;
    let mut end: Option<usize> = None;
    for (i, line) in lines.iter().enumerate() {
        let trimmed = line.trim();
        if trimmed == format!("[{}]", section) {
            start = Some(i);
            continue;
        }
        if start.is_some() && end.is_none() && trimmed.starts_with('[') {
            end = Some(i);
        }
    }

    let Some(mut s) = start else {
        return Ok(());
    };
    let e = end.unwrap_or(lines.len());

    // Also remove blank line before section
    if s > 0 && lines[s - 1].trim().is_empty() {
        s -= 1;
    }

    let mut result_lines: Vec<&str> = Vec::new();
    result_lines.extend_from_slice(&lines[..s]);
    result_lines.extend_from_slice(&lines[e..]);

    fs::write(path, result_lines.join("\n")).context("writing INI file")?;
    Ok(())
}

/// Save an AWS profile to ~/.aws/credentials and ~/.aws/config.
pub fn save_profile(name: &str, access_key: &str, secret_key: &str, region: &str) -> Result<()> {
    let dir = aws_dir();
    fs::create_dir_all(&dir).context("creating ~/.aws")?;

    let mut cred_values: Vec<(&str, &str)> = vec![("aws_access_key_id", access_key)];
    if !secret_key.is_empty() {
        cred_values.push(("aws_secret_access_key", secret_key));
    }
    update_ini_section(&dir.join("credentials"), name, &cred_values)?;

    if !region.is_empty() {
        let config_section = if name == "default" {
            "default".to_string()
        } else {
            format!("profile {}", name)
        };
        update_ini_section(&dir.join("config"), &config_section, &[("region", region)])?;
    }

    Ok(())
}

/// Delete an AWS profile from ~/.aws/credentials and ~/.aws/config.
pub fn delete_profile(name: &str) -> Result<()> {
    let dir = aws_dir();
    let _ = remove_ini_section(&dir.join("credentials"), name);
    let config_section = if name == "default" {
        "default".to_string()
    } else {
        format!("profile {}", name)
    };
    let _ = remove_ini_section(&dir.join("config"), &config_section);
    Ok(())
}

/// Read profile details (access key, region) for pre-filling edit form.
pub fn read_profile_detail(name: &str) -> (String, String) {
    let dir = aws_dir();
    let access_key =
        read_ini_value(&dir.join("credentials"), name, "aws_access_key_id").unwrap_or_default();
    let config_section = if name == "default" {
        "default".to_string()
    } else {
        format!("profile {}", name)
    };
    let region = read_ini_value(&dir.join("config"), &config_section, "region").unwrap_or_default();
    (access_key, region)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;

    #[test]
    fn read_ini_value_found() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(
            &path,
            "[default]\naws_access_key_id = AKIA123\nregion = us-east-1\n",
        )
        .unwrap();
        assert_eq!(
            read_ini_value(&path, "default", "aws_access_key_id"),
            Some("AKIA123".into())
        );
        assert_eq!(
            read_ini_value(&path, "default", "region"),
            Some("us-east-1".into())
        );
    }

    #[test]
    fn read_ini_value_missing_key() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(&path, "[default]\nregion = us-east-1\n").unwrap();
        assert_eq!(read_ini_value(&path, "default", "aws_access_key_id"), None);
    }

    #[test]
    fn read_ini_value_missing_section() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(&path, "[other]\nkey = val\n").unwrap();
        assert_eq!(read_ini_value(&path, "default", "key"), None);
    }

    #[test]
    fn read_ini_value_missing_file() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("nonexistent.ini");
        assert_eq!(read_ini_value(&path, "default", "key"), None);
    }

    #[test]
    fn read_ini_value_multiple_sections() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(
            &path,
            "[default]\nregion = us-east-1\n\n[profile dev]\nregion = eu-west-1\n",
        )
        .unwrap();
        assert_eq!(
            read_ini_value(&path, "profile dev", "region"),
            Some("eu-west-1".into())
        );
    }

    #[test]
    fn update_ini_section_new_file() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("new.ini");
        update_ini_section(&path, "default", &[("key", "val")]).unwrap();
        let content = fs::read_to_string(&path).unwrap();
        assert!(content.contains("[default]"));
        assert!(content.contains("key = val"));
    }

    #[test]
    fn update_ini_section_new_section() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(&path, "[existing]\nfoo = bar\n").unwrap();
        update_ini_section(&path, "new-section", &[("baz", "qux")]).unwrap();
        let content = fs::read_to_string(&path).unwrap();
        assert!(content.contains("[existing]"));
        assert!(content.contains("[new-section]"));
        assert!(content.contains("baz = qux"));
    }

    #[test]
    fn update_ini_section_merge_existing() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(&path, "[default]\nold_key = old_val\n").unwrap();
        update_ini_section(
            &path,
            "default",
            &[("old_key", "new_val"), ("new_key", "added")],
        )
        .unwrap();
        let content = fs::read_to_string(&path).unwrap();
        assert!(content.contains("old_key = new_val"));
        assert!(content.contains("new_key = added"));
    }

    #[test]
    fn update_ini_section_preserves_other_sections() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(
            &path,
            "[first]\na = 1\n\n[second]\nb = 2\n\n[third]\nc = 3\n",
        )
        .unwrap();
        update_ini_section(&path, "second", &[("b", "updated")]).unwrap();
        let content = fs::read_to_string(&path).unwrap();
        assert!(content.contains("a = 1"));
        assert!(content.contains("b = updated"));
        assert!(content.contains("c = 3"));
    }

    #[test]
    fn remove_ini_section_middle() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(
            &path,
            "[first]\na = 1\n\n[second]\nb = 2\n\n[third]\nc = 3\n",
        )
        .unwrap();
        remove_ini_section(&path, "second").unwrap();
        let content = fs::read_to_string(&path).unwrap();
        assert!(content.contains("[first]"));
        assert!(!content.contains("[second]"));
        assert!(content.contains("[third]"));
    }

    #[test]
    fn remove_ini_section_last() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(&path, "[first]\na = 1\n\n[second]\nb = 2\n").unwrap();
        remove_ini_section(&path, "second").unwrap();
        let content = fs::read_to_string(&path).unwrap();
        assert!(content.contains("[first]"));
        assert!(!content.contains("[second]"));
    }

    #[test]
    fn remove_ini_section_nonexistent() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(&path, "[default]\na = 1\n").unwrap();
        remove_ini_section(&path, "nope").unwrap();
        let content = fs::read_to_string(&path).unwrap();
        assert!(content.contains("[default]"));
    }

    #[test]
    fn save_and_read_profile() {
        let dir = TempDir::new().unwrap();
        let cred_path = dir.path().join("credentials");
        let config_path = dir.path().join("config");

        // save_profile uses aws_dir() which points to ~/.aws, so test the building blocks directly
        update_ini_section(
            &cred_path,
            "myprofile",
            &[
                ("aws_access_key_id", "AKIATEST"),
                ("aws_secret_access_key", "SECRET"),
            ],
        )
        .unwrap();
        update_ini_section(
            &config_path,
            "profile myprofile",
            &[("region", "ap-southeast-1")],
        )
        .unwrap();

        assert_eq!(
            read_ini_value(&cred_path, "myprofile", "aws_access_key_id"),
            Some("AKIATEST".into())
        );
        assert_eq!(
            read_ini_value(&config_path, "profile myprofile", "region"),
            Some("ap-southeast-1".into())
        );
    }

    #[test]
    fn save_default_profile_no_prefix() {
        let dir = TempDir::new().unwrap();
        let config_path = dir.path().join("config");
        // default profile uses [default], not [profile default]
        update_ini_section(&config_path, "default", &[("region", "us-west-2")]).unwrap();
        assert_eq!(
            read_ini_value(&config_path, "default", "region"),
            Some("us-west-2".into())
        );
    }

    #[test]
    fn read_ini_value_with_spaces_around_equals() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.ini");
        fs::write(&path, "[default]\n  key  =  value  \n").unwrap();
        assert_eq!(
            read_ini_value(&path, "default", "key"),
            Some("value".into())
        );
    }
}
