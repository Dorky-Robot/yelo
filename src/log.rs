use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::PathBuf;
use std::time::SystemTime;

fn log_path() -> PathBuf {
    dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".yelo/yelo.log")
}

pub fn init() {
    let path = log_path();
    if let Some(parent) = path.parent() {
        let _ = fs::create_dir_all(parent);
    }
    let _ = fs::write(&path, "");
}

pub fn log(msg: &str) {
    let path = log_path();
    if let Ok(mut f) = OpenOptions::new().create(true).append(true).open(&path) {
        let elapsed = SystemTime::now()
            .duration_since(SystemTime::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        let _ = writeln!(f, "[{elapsed}] {msg}");
    }
}
