/// Persistent background daemon that polls pending Glacier restores
/// and auto-downloads them when available.
///
/// PID file: ~/.yelo/daemon.pid
/// Log file: ~/.yelo/daemon.log
use std::fs;
use std::io::Write;
use std::path::PathBuf;
use std::process::Command;
use std::time::SystemTime;

use anyhow::{Context, Result};

use crate::{cache, config::Config, restore, restore::RestoreStatus};

fn yelo_dir() -> PathBuf {
    dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".yelo")
}

fn pid_path() -> PathBuf {
    yelo_dir().join("daemon.pid")
}

fn log_path() -> PathBuf {
    yelo_dir().join("daemon.log")
}

fn daemon_log(msg: &str) {
    let path = log_path();
    if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&path) {
        let elapsed = SystemTime::now()
            .duration_since(SystemTime::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        let _ = writeln!(f, "[{elapsed}] {msg}");
    }
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

pub fn is_running() -> bool {
    read_pid().is_some()
}

pub fn start() -> Result<()> {
    if let Some(pid) = read_pid() {
        eprintln!("daemon already running (pid {})", pid);
        return Ok(());
    }

    let exe = std::env::current_exe().context("finding yelo binary")?;
    let log_file = fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(log_path())
        .context("opening daemon log")?;
    let log_err = log_file.try_clone().context("cloning log handle")?;

    let child = Command::new(exe)
        .arg("daemon")
        .arg("run")
        .stdout(log_file)
        .stderr(log_err)
        .stdin(std::process::Stdio::null())
        .spawn()
        .context("spawning daemon process")?;

    let pid = child.id();
    fs::create_dir_all(yelo_dir()).context("creating ~/.yelo")?;
    fs::write(pid_path(), pid.to_string()).context("writing PID file")?;

    eprintln!("daemon started (pid {})", pid);
    Ok(())
}

pub fn stop() -> Result<()> {
    let Some(pid) = read_pid() else {
        eprintln!("daemon is not running");
        return Ok(());
    };

    // Send SIGTERM
    unsafe {
        libc_kill(pid);
    }

    let _ = fs::remove_file(pid_path());
    eprintln!("daemon stopped (pid {})", pid);
    Ok(())
}

pub fn status() {
    match read_pid() {
        Some(pid) => eprintln!("daemon running (pid {})", pid),
        None => eprintln!("daemon is not running"),
    }
}

/// The actual daemon loop — called by `yelo daemon run`.
/// This runs in the foreground of the spawned process.
pub fn run_loop() -> Result<()> {
    let config = Config::load().unwrap_or_default();
    let interval = config.daemon_poll_interval();
    daemon_log(&format!("daemon started (poll interval: {}s)", interval));

    loop {
        std::thread::sleep(std::time::Duration::from_secs(interval));
        poll_restores();
    }
}

// ---------------------------------------------------------------------------
// Internal
// ---------------------------------------------------------------------------

fn poll_restores() {
    let restores = restore::list_requests();
    let pending: Vec<_> = restores
        .iter()
        .filter(|r| r.status == RestoreStatus::Pending)
        .collect();

    if pending.is_empty() {
        return;
    }

    daemon_log(&format!("checking {} pending restores", pending.len()));

    for req in pending {
        match restore::check_restore(req) {
            Ok(RestoreStatus::Available) => {
                daemon_log(&format!("{}: available, downloading {}", req.id, req.key));
                let _ = restore::update_status(&req.id, RestoreStatus::Available, None);

                match cache::download_to_cache(&req.bucket, &req.key, &req.region, &req.profile) {
                    Ok(_) => {
                        let _ = restore::update_status(&req.id, RestoreStatus::Downloaded, None);
                        daemon_log(&format!("{}: downloaded", req.id));
                    }
                    Err(e) => {
                        daemon_log(&format!("{}: download failed: {}", req.id, e));
                    }
                }
            }
            Ok(status) => {
                daemon_log(&format!("{}: still {}", req.id, status));
            }
            Err(e) => {
                daemon_log(&format!("{}: check failed: {}", req.id, e));
            }
        }
    }
}

fn read_pid() -> Option<u32> {
    let data = fs::read_to_string(pid_path()).ok()?;
    let pid: u32 = data.trim().parse().ok()?;

    // Check if process is actually alive (kill with signal 0)
    let alive = unsafe { libc_kill_check(pid) };
    if alive {
        Some(pid)
    } else {
        // Stale PID file — clean up
        let _ = fs::remove_file(pid_path());
        None
    }
}

// Minimal libc wrappers to avoid adding a libc dependency
unsafe extern "C" {
    fn kill(pid: i32, sig: i32) -> i32;
}

unsafe fn libc_kill(pid: u32) {
    unsafe {
        kill(pid as i32, 15); // SIGTERM
    }
}

unsafe fn libc_kill_check(pid: u32) -> bool {
    unsafe { kill(pid as i32, 0) == 0 }
}
