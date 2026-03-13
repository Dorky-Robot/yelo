/// CLI subcommands — the non-TUI interface.
use std::io::{self, Write};
use std::path::Path;

use anyhow::{Context, Result, bail};
use clap::{Parser, Subcommand};

use crate::aws_ops;
use crate::config::Config;
use crate::helpers::{is_glacier, parse_bucket_path, resolve_path, resolve_prefix, validate_tier};
use crate::output;
use crate::state::State;

// ---------------------------------------------------------------------------
// Clap definitions
// ---------------------------------------------------------------------------

#[derive(Parser)]
#[command(
    name = "yelo",
    version,
    about = "FTP-style CLI for Amazon S3 and Glacier"
)]
pub struct Cli {
    /// Override bucket resolution
    #[arg(long, global = true)]
    pub bucket: Option<String>,
    /// Override AWS region
    #[arg(long, global = true)]
    pub region: Option<String>,
    /// Override AWS profile
    #[arg(long, global = true)]
    pub profile: Option<String>,

    #[command(subcommand)]
    pub command: Option<Command>,
}

#[derive(Subcommand, Clone)]
pub enum Command {
    /// Interactive terminal UI
    Tui,
    /// Print working directory (bucket:/prefix/)
    Pwd,
    /// Change directory
    Cd {
        /// Target path (supports bucket:path syntax)
        path: String,
    },
    /// List objects and prefixes
    Ls {
        /// Long format (class, size, date, key)
        #[arg(short)]
        l: bool,
        /// Recursive listing
        #[arg(short = 'R')]
        recursive: bool,
        /// Path to list
        path: Option<String>,
    },
    /// Show object metadata
    Stat {
        /// Object path
        path: String,
    },
    /// Download an object
    Get {
        /// Remote object key
        remote: String,
        /// Local destination (- for stdout)
        local: Option<String>,
    },
    /// Upload an object
    Put {
        /// Local file to upload
        local: String,
        /// Remote key (defaults to current prefix + filename)
        remote: Option<String>,
        /// S3 storage class
        #[arg(long, default_value = "DEEP_ARCHIVE")]
        storage_class: String,
    },
    /// Initiate Glacier restore
    Restore {
        /// Object path
        path: String,
        /// Days to keep restored copy
        #[arg(long, default_value_t = 7)]
        days: i32,
        /// Retrieval tier (Standard, Bulk, Expedited)
        #[arg(long, default_value = "Standard")]
        tier: String,
    },
    /// Archive a file to Glacier Deep Archive
    Freeze {
        /// Local file to upload
        local: String,
        /// Remote key (defaults to current prefix + filename)
        remote: Option<String>,
    },
    /// Restore a frozen object from Glacier
    Thaw {
        /// Object path
        path: String,
        /// Days to keep restored copy
        #[arg(long, default_value_t = 7)]
        days: i32,
        /// Retrieval tier (Standard, Bulk, Expedited)
        #[arg(long, default_value = "Standard")]
        tier: String,
    },
    /// Manage configured buckets
    Buckets {
        #[command(subcommand)]
        action: Option<BucketsAction>,
    },
    /// Manage the background auto-download daemon
    Daemon {
        #[command(subcommand)]
        action: DaemonAction,
    },
}

#[derive(Subcommand, Clone)]
pub enum BucketsAction {
    /// List configured buckets
    List {
        /// List remote buckets from AWS
        #[arg(long)]
        remote: bool,
    },
    /// Add a bucket to config
    Add {
        /// Bucket name
        name: String,
        /// AWS region for this bucket
        #[arg(long)]
        region: Option<String>,
        /// AWS profile for this bucket
        #[arg(long)]
        profile: Option<String>,
    },
    /// Remove a bucket from config
    Remove {
        /// Bucket name
        name: String,
    },
    /// Get or set the default bucket
    Default {
        /// Bucket name to set as default
        name: Option<String>,
    },
}

#[derive(Subcommand, Clone)]
pub enum DaemonAction {
    /// Start the daemon in the background
    Start,
    /// Stop the running daemon
    Stop,
    /// Show daemon status
    Status,
    /// Run the daemon loop (internal — used by `daemon start`)
    #[command(hide = true)]
    Run,
}

// ---------------------------------------------------------------------------
// Runtime environment
// ---------------------------------------------------------------------------

struct Env {
    cfg: Config,
    st: State,
    bucket_flag: String,
    region_flag: String,
    profile_flag: String,
}

impl Env {
    fn new(cli: &Cli) -> Result<Self> {
        let cfg = Config::load().unwrap_or_default();
        let st = State::load().unwrap_or_default();
        Ok(Env {
            cfg,
            st,
            bucket_flag: cli.bucket.clone().unwrap_or_default(),
            region_flag: cli.region.clone().unwrap_or_default(),
            profile_flag: cli.profile.clone().unwrap_or_default(),
        })
    }

    /// Resolve bucket from flag → state → config default → sole configured.
    fn resolve_bucket(&self) -> Result<String> {
        if !self.bucket_flag.is_empty() {
            return Ok(self.bucket_flag.clone());
        }
        if !self.st.bucket.is_empty() {
            return Ok(self.st.bucket.clone());
        }
        self.cfg
            .resolve_bucket()
            .context("no bucket set — use --bucket, cd bucket:, or buckets default")
    }

    /// Resolve region: flag → per-bucket config → global config → empty (SDK default).
    fn resolve_region(&self, bucket: &str) -> String {
        if !self.region_flag.is_empty() {
            return self.region_flag.clone();
        }
        self.cfg.resolve_region(bucket)
    }

    /// Resolve profile: flag → per-bucket config → global config → empty (SDK default).
    fn resolve_profile(&self, bucket: &str) -> String {
        if !self.profile_flag.is_empty() {
            return self.profile_flag.clone();
        }
        self.cfg.resolve_profile(bucket)
    }
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

pub fn run(cli: Cli, cmd: Command) -> Result<()> {
    let mut env = Env::new(&cli)?;
    match cmd {
        Command::Tui => unreachable!("tui handled in main"),
        Command::Pwd => run_pwd(&env),
        Command::Cd { path } => run_cd(&mut env, &path),
        Command::Ls { l, recursive, path } => run_ls(&env, l, recursive, path.as_deref()),
        Command::Stat { path } => run_stat(&env, &path),
        Command::Get { remote, local } => run_get(&env, &remote, local.as_deref()),
        Command::Put {
            local,
            remote,
            storage_class,
        } => run_put(&env, &local, remote.as_deref(), &storage_class),
        Command::Restore { path, days, tier } => run_restore(&env, &path, days, &tier),
        Command::Freeze { local, remote } => {
            run_put(&env, &local, remote.as_deref(), "DEEP_ARCHIVE")
        }
        Command::Thaw { path, days, tier } => run_restore(&env, &path, days, &tier),
        Command::Buckets { action } => run_buckets(&mut env, action),
        Command::Daemon { .. } => unreachable!("daemon handled in main"),
    }
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

fn run_pwd(env: &Env) -> Result<()> {
    let bucket = match env.resolve_bucket() {
        Ok(b) => b,
        Err(_) => {
            println!("(no bucket set)");
            return Ok(());
        }
    };
    println!("{}:/{}", bucket, env.st.prefix);
    Ok(())
}

fn run_cd(env: &mut Env, path: &str) -> Result<()> {
    let (bucket_part, path_part) = parse_bucket_path(path);

    if !bucket_part.is_empty() {
        env.st.bucket = bucket_part.to_string();
        env.st.prefix.clear();
    }

    let target = if path_part.is_empty() && !bucket_part.is_empty() {
        "/"
    } else {
        path_part
    };

    if !target.is_empty() {
        let resolved = resolve_prefix(&env.st.prefix, target);
        env.st.prefix = resolved;
    }

    env.st.save()?;
    eprintln!("/{}", env.st.prefix);
    Ok(())
}

fn run_ls(env: &Env, long: bool, recursive: bool, path: Option<&str>) -> Result<()> {
    let bucket = env.resolve_bucket()?;
    let region = env.resolve_region(&bucket);
    let profile = env.resolve_profile(&bucket);

    let prefix = match path {
        Some(p) => resolve_prefix(&env.st.prefix, p),
        None => env.st.prefix.clone(),
    };

    let items = if recursive {
        aws_ops::list_objects_recursive(&bucket, &prefix, &region, &profile)?
    } else {
        aws_ops::list_objects(&bucket, &prefix, &region, &profile)?
    };

    if items.is_empty() {
        eprintln!("no objects found");
        return Ok(());
    }

    let tty = output::is_tty();
    let mut out = io::stdout().lock();
    output::list_objects(&mut out, &items, &prefix, long, tty)?;
    Ok(())
}

fn run_stat(env: &Env, path: &str) -> Result<()> {
    let bucket = env.resolve_bucket()?;
    let region = env.resolve_region(&bucket);
    let profile = env.resolve_profile(&bucket);

    let key = resolve_path(&env.st.prefix, path);
    if key.is_empty() || key.ends_with('/') {
        bail!("stat requires an object key, not a directory");
    }

    let info = aws_ops::head_object(&bucket, &key, &region, &profile)?;
    let tty = output::is_tty();
    let mut out = io::stdout().lock();
    output::format_stat(&mut out, &info, tty)?;
    Ok(())
}

fn run_get(env: &Env, remote: &str, local: Option<&str>) -> Result<()> {
    let bucket = env.resolve_bucket()?;
    let region = env.resolve_region(&bucket);
    let profile = env.resolve_profile(&bucket);

    let key = resolve_path(&env.st.prefix, remote);
    if key.is_empty() || key.ends_with('/') {
        bail!("get requires an object key, not a directory");
    }

    // Pre-download Glacier check
    let info = aws_ops::head_object(&bucket, &key, &region, &profile)?;
    if is_glacier(&info.storage_class) && info.restore_status != "available" {
        if info.restore_status == "in-progress" {
            bail!("object is being restored from Glacier — try again later");
        } else {
            bail!(
                "object is in {} — run `yelo restore {}` first",
                info.storage_class,
                key
            );
        }
    }

    let to_stdout = local == Some("-");

    if to_stdout {
        let data = aws_ops::download_bytes(&bucket, &key, &region, &profile)?;
        io::stdout().write_all(&data)?;
    } else {
        let dest = local.map(|s| s.to_string()).unwrap_or_else(|| {
            Path::new(&key)
                .file_name()
                .unwrap_or_default()
                .to_string_lossy()
                .to_string()
        });
        aws_ops::download_to_path(&bucket, &key, &region, &profile, Path::new(&dest))?;
        eprintln!("{} → {}", key, dest);
    }
    Ok(())
}

fn run_put(env: &Env, local: &str, remote: Option<&str>, storage_class: &str) -> Result<()> {
    let bucket = env.resolve_bucket()?;
    let region = env.resolve_region(&bucket);
    let profile = env.resolve_profile(&bucket);

    let local_path = Path::new(local);
    if !local_path.exists() {
        bail!("file not found: {}", local);
    }

    let key = match remote {
        Some(r) => resolve_path(&env.st.prefix, r),
        None => {
            let name = local_path.file_name().unwrap_or_default().to_string_lossy();
            resolve_path(&env.st.prefix, &name)
        }
    };

    aws_ops::upload(&bucket, &key, local_path, storage_class, &region, &profile)?;
    eprintln!("{} → s3://{}/{} ({})", local, bucket, key, storage_class);
    Ok(())
}

fn run_restore(env: &Env, path: &str, days: i32, tier: &str) -> Result<()> {
    let bucket = env.resolve_bucket()?;
    let region = env.resolve_region(&bucket);
    let profile = env.resolve_profile(&bucket);

    let key = resolve_path(&env.st.prefix, path);
    if key.is_empty() || key.ends_with('/') {
        bail!("restore requires an object key, not a directory");
    }

    let info = aws_ops::head_object(&bucket, &key, &region, &profile)?;
    if !is_glacier(&info.storage_class) {
        bail!(
            "object is {} — restore only applies to Glacier classes",
            info.storage_class
        );
    }

    validate_tier(&info.storage_class, tier).map_err(|e| anyhow::anyhow!(e))?;

    if info.restore_status == "in-progress" {
        eprintln!("restore already in progress: {}", key);
        return Ok(());
    }
    if info.restore_status == "available" {
        eprintln!("already restored: {}", key);
        return Ok(());
    }

    aws_ops::restore_object(&bucket, &key, days, tier, &region, &profile)?;
    eprintln!("restore initiated: {} (tier={}, days={})", key, tier, days);
    Ok(())
}

fn run_buckets(env: &mut Env, action: Option<BucketsAction>) -> Result<()> {
    let action = action.unwrap_or(BucketsAction::List { remote: false });

    match action {
        BucketsAction::List { remote } => {
            if remote || env.cfg.buckets.is_empty() {
                let region = env.resolve_region("");
                let profile = env.resolve_profile("");
                let region = if region.is_empty() {
                    "us-east-1".to_string()
                } else {
                    region
                };
                let buckets = aws_ops::list_buckets(&region, &profile)?;
                for name in &buckets {
                    println!("{}", name);
                }
            } else {
                for b in &env.cfg.buckets {
                    let marker = if b.name == env.cfg.default_bucket {
                        "* "
                    } else {
                        "  "
                    };
                    let mut extras = Vec::new();
                    if !b.region.is_empty() {
                        extras.push(format!("region={}", b.region));
                    }
                    if !b.profile.is_empty() {
                        extras.push(format!("profile={}", b.profile));
                    }
                    if extras.is_empty() {
                        println!("{}{}", marker, b.name);
                    } else {
                        println!("{}{} ({})", marker, b.name, extras.join(", "));
                    }
                }
            }
        }
        BucketsAction::Add {
            name,
            region,
            profile,
        } => {
            env.cfg.add_bucket(
                &name,
                &region.unwrap_or_default(),
                &profile.unwrap_or_default(),
            );
            env.cfg.save()?;
            eprintln!("added bucket {}", name);
        }
        BucketsAction::Remove { name } => {
            if !env.cfg.remove_bucket(&name) {
                bail!("bucket '{}' not in config", name);
            }
            if env.st.bucket == name {
                env.st.bucket.clear();
                env.st.prefix.clear();
                env.st.save()?;
            }
            env.cfg.save()?;
            eprintln!("removed bucket {}", name);
        }
        BucketsAction::Default { name } => {
            if let Some(name) = name {
                if !env.cfg.set_default(&name) {
                    bail!("bucket '{}' not in config — add it first", name);
                }
                env.cfg.save()?;
                eprintln!("default bucket set to {}", name);
            } else if env.cfg.default_bucket.is_empty() {
                println!("(no default bucket set)");
            } else {
                println!("{}", env.cfg.default_bucket);
            }
        }
    }
    Ok(())
}
