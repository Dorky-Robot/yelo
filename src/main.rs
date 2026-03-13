mod app;
mod aws_ops;
mod cache;
mod cli;
mod config;
mod credentials;
mod daemon;
mod helpers;
mod log;
mod output;
mod restore;
mod state;
mod ui;

use std::io::{self, stdout};
use std::time::Duration;

use anyhow::Result;
use clap::Parser;
use crossterm::ExecutableCommand;
use crossterm::event::{self, Event, KeyCode, KeyEventKind};
use crossterm::terminal::{
    EnterAlternateScreen, LeaveAlternateScreen, disable_raw_mode, enable_raw_mode,
};
use ratatui::Terminal;
use ratatui::backend::CrosstermBackend;

use app::{App, ConfirmAction, Mode, Tab};
use helpers::{is_glacier, is_image, parent_prefix};

const TIER_OPTIONS: &[&str] = &["Standard", "Bulk", "Expedited"];

fn main() -> Result<()> {
    let parsed = cli::Cli::parse();

    match &parsed.command {
        None | Some(cli::Command::Tui) => launch_tui(),
        Some(cli::Command::Daemon { action }) => match action {
            cli::DaemonAction::Start => daemon::start(),
            cli::DaemonAction::Stop => daemon::stop(),
            cli::DaemonAction::Status => {
                daemon::status();
                Ok(())
            }
            cli::DaemonAction::Run => daemon::run_loop(),
        },
        Some(_) => {
            let cmd = parsed.command.clone().unwrap();
            cli::run(parsed, cmd)
        }
    }
}

fn launch_tui() -> Result<()> {
    log::init();
    log::log("yelo starting TUI");

    // Query terminal for image protocol support before entering raw mode
    let picker = ratatui_image::picker::Picker::from_query_stdio()
        .unwrap_or_else(|_| ratatui_image::picker::Picker::halfblocks());

    enable_raw_mode()?;
    stdout().execute(EnterAlternateScreen)?;

    let backend = CrosstermBackend::new(stdout());
    let mut terminal = Terminal::new(backend)?;
    terminal.clear()?;

    let mut app = App::new(picker);
    let result = run_loop(&mut terminal, &mut app);

    disable_raw_mode()?;
    io::stdout().execute(LeaveAlternateScreen)?;

    if let Err(ref e) = result {
        eprintln!("yelo: {}", e);
    }

    Ok(())
}

fn run_loop(terminal: &mut Terminal<CrosstermBackend<io::Stdout>>, app: &mut App) -> Result<()> {
    // Read terminal events in a background thread to prevent blocking.
    // crossterm's event::read() can block indefinitely (e.g. leftover bytes
    // from the image picker terminal query), which would freeze the spinner.
    let (event_tx, event_rx) = std::sync::mpsc::channel();
    std::thread::spawn(move || {
        while let Ok(evt) = event::read() {
            if event_tx.send(evt).is_err() {
                break;
            }
        }
    });

    loop {
        app.spinner_tick = app.spinner_tick.wrapping_add(1);
        app.poll_bg();

        terminal.draw(|f| ui::draw(f, app))?;

        if let Ok(evt) = event_rx.recv_timeout(Duration::from_millis(100))
            && let Event::Key(key) = evt
        {
            if key.kind != KeyEventKind::Press {
                continue;
            }
            // Allow quit even while loading
            if app.loading.is_some() {
                if key.code == KeyCode::Char('q') {
                    app.should_quit = true;
                }
                continue;
            }
            handle_key(app, key.code);
        }

        if app.should_quit {
            app.save_state();
            return Ok(());
        }
    }
}

// ---------------------------------------------------------------------------
// Key dispatch
// ---------------------------------------------------------------------------

fn handle_key(app: &mut App, code: KeyCode) {
    match &app.mode {
        Mode::Normal if app.tab == Tab::Browse && app.submenu => handle_browse_submenu(app, code),
        Mode::Normal if app.tab == Tab::Browse => handle_browse(app, code),
        Mode::Normal if app.tab == Tab::Profiles && app.submenu => {
            handle_profiles_submenu(app, code)
        }
        Mode::Normal if app.tab == Tab::Profiles => handle_profiles(app, code),
        Mode::Normal if app.tab == Tab::Restores && app.submenu => {
            handle_restores_submenu(app, code)
        }
        Mode::Normal if app.tab == Tab::Restores => handle_restores(app, code),
        Mode::Normal if app.tab == Tab::Library && app.submenu => handle_library_submenu(app, code),
        Mode::Normal if app.tab == Tab::Library => handle_library(app, code),
        Mode::Normal => {}
        Mode::Help => {
            app.mode = Mode::Normal;
        }
        Mode::BucketPicker { .. } => handle_bucket_picker(app, code),
        Mode::Confirm { .. } => handle_confirm(app, code),
        Mode::Filter => handle_filter(app, code),
        Mode::Detail { .. } => handle_detail(app, code),
        Mode::LinkBucket { .. } => handle_link_form(app, code),
        Mode::AddProfile { .. } | Mode::EditProfile { .. } => handle_profile_form(app, code),
        Mode::TierPicker { .. } => handle_tier_picker(app, code),
        Mode::ImagePreview { .. } => handle_image_preview(app, code),
        Mode::Rename { .. } => handle_rename(app, code),
        Mode::LibraryInfo { .. } => handle_library_info(app, code),
        Mode::Upload { .. } => handle_upload_form(app, code),
    }
}

// ---------------------------------------------------------------------------
// Tab switching (shared across normal modes)
// ---------------------------------------------------------------------------

fn try_switch_tab(app: &mut App, code: KeyCode) -> bool {
    let tabs = [Tab::Browse, Tab::Profiles, Tab::Restores, Tab::Library];
    let new_tab = match code {
        KeyCode::Char('1') => Some(Tab::Browse),
        KeyCode::Char('2') => Some(Tab::Profiles),
        KeyCode::Char('3') => Some(Tab::Restores),
        KeyCode::Char('4') => Some(Tab::Library),
        KeyCode::Right => {
            let idx = tabs.iter().position(|t| *t == app.tab).unwrap_or(0);
            Some(tabs[(idx + 1) % tabs.len()])
        }
        KeyCode::Left => {
            let idx = tabs.iter().position(|t| *t == app.tab).unwrap_or(0);
            Some(tabs[(idx + tabs.len() - 1) % tabs.len()])
        }
        _ => None,
    };
    if let Some(tab) = new_tab {
        app.tab = tab;
        app.submenu = false;
        if tab == Tab::Profiles && app.profiles.is_empty() {
            app.loading = Some("Loading profiles...".into());
            app.spawn_load_profiles();
        }
        if tab == Tab::Restores {
            app.restores = restore::list_requests();
        }
        if tab == Tab::Library {
            app.refresh_library();
        }
        true
    } else {
        false
    }
}

// ---------------------------------------------------------------------------
// Browse handlers
// ---------------------------------------------------------------------------

fn handle_browse(app: &mut App, code: KeyCode) {
    if try_switch_tab(app, code) {
        return;
    }
    match code {
        KeyCode::Char('q') => app.should_quit = true,
        KeyCode::Esc | KeyCode::Char('h') | KeyCode::Backspace => {
            if !app.prefix.is_empty() {
                app.prefix = parent_prefix(&app.prefix);
                app.loading = Some("Loading...".into());
                app.spawn_list_objects();
            } else {
                app.should_quit = true;
            }
        }
        KeyCode::Char('j') | KeyCode::Down => app.move_down(),
        KeyCode::Char('k') | KeyCode::Up => app.move_up(),
        KeyCode::Enter | KeyCode::Char('l') => {
            let filtered = app.filtered_items();
            if let Some(item) = filtered.get(app.browse_selected) {
                if item.is_prefix {
                    app.prefix = item.key.clone();
                    app.loading = Some("Loading...".into());
                    app.spawn_list_objects();
                } else if is_image(&item.key) && !is_glacier(&item.storage_class) {
                    let key = item.key.clone();
                    log::log(&format!(
                        "Image detected: key={:?} cached={}",
                        key,
                        cache::is_cached(&app.bucket, &key)
                    ));
                    if cache::is_cached(&app.bucket, &key) {
                        if app.load_image_preview(&key) {
                            app.mode = Mode::ImagePreview { key };
                        } else {
                            // Preview failed, fall back to detail
                            app.loading = Some("Loading metadata...".into());
                            app.spawn_head_object(&key);
                        }
                    } else {
                        let name = key.rsplit('/').next().unwrap_or(&key).to_string();
                        app.loading = Some(format!("Caching {}...", name));
                        app.spawn_cache_for_preview(&key);
                    }
                } else {
                    let key = item.key.clone();
                    app.loading = Some("Loading metadata...".into());
                    app.spawn_head_object(&key);
                }
            }
        }
        KeyCode::Char('o') => {
            begin_open(app);
        }
        KeyCode::Char('/') => {
            app.mode = Mode::Filter;
            app.filter_text.clear();
        }
        KeyCode::Char('.') => app.submenu = true,
        _ => {}
    }
}

fn begin_open(app: &mut App) {
    let filtered = app.filtered_items();
    if let Some(item) = filtered.get(app.browse_selected) {
        if item.is_prefix {
            return;
        }
        if is_glacier(&item.storage_class) {
            app.flash("Object is in Glacier — restore it first (r)".into());
            return;
        }
        let key = item.key.clone();
        let name = key.rsplit('/').next().unwrap_or(&key).to_string();
        if cache::is_cached(&app.bucket, &key) {
            let _ = cache::open_cached(&app.bucket, &key);
            app.flash(format!("Opened {} (cached)", name));
        } else {
            app.loading = Some(format!("Caching {}...", name));
            app.spawn_cache_and_open(&key);
        }
    }
}

fn handle_browse_submenu(app: &mut App, code: KeyCode) {
    match code {
        KeyCode::Char('.') | KeyCode::Esc => app.submenu = false,
        KeyCode::Char('g') => {
            app.submenu = false;
            begin_download(app);
        }
        KeyCode::Char('r') => {
            app.submenu = false;
            begin_restore(app);
        }
        KeyCode::Char('s') => {
            app.submenu = false;
            let filtered = app.filtered_items();
            if let Some(item) = filtered.get(app.browse_selected)
                && !item.is_prefix
            {
                let key = item.key.clone();
                app.loading = Some("Loading metadata...".into());
                app.spawn_head_object(&key);
            }
        }
        KeyCode::Char('b') => {
            app.submenu = false;
            app.loading = Some("Loading buckets...".into());
            app.spawn_list_buckets();
        }
        KeyCode::Char('u') => {
            app.submenu = false;
            if app.bucket.is_empty() {
                app.flash("No bucket selected".into());
                return;
            }
            app.mode = Mode::Upload {
                focus: 0,
                local_path: String::new(),
                key: app.prefix.clone(),
                storage_class: "DEEP_ARCHIVE".into(),
                completions: Vec::new(),
                comp_selected: None,
            };
        }
        KeyCode::Char('R') => {
            app.submenu = false;
            app.loading = Some("Refreshing...".into());
            app.spawn_list_objects();
        }
        KeyCode::Char('?') => {
            app.mode = Mode::Help;
            app.submenu = false;
        }
        _ => {}
    }
}

fn begin_download(app: &mut App) {
    let filtered = app.filtered_items();
    if let Some(item) = filtered.get(app.browse_selected) {
        if item.is_prefix {
            return;
        }
        if is_glacier(&item.storage_class) {
            app.flash("Object is in Glacier — restore it first (r)".into());
            return;
        }
        app.mode = Mode::Confirm {
            action: ConfirmAction::Download,
            target: item.key.clone(),
        };
    }
}

fn begin_restore(app: &mut App) {
    let filtered = app.filtered_items();
    if let Some(item) = filtered.get(app.browse_selected) {
        if item.is_prefix {
            return;
        }
        if !is_glacier(&item.storage_class) {
            app.flash("Not a Glacier object".into());
            return;
        }
        app.mode = Mode::TierPicker {
            key: item.key.clone(),
            storage_class: item.storage_class.clone(),
            selected: 0,
        };
    }
}

// ---------------------------------------------------------------------------
// Profiles handlers
// ---------------------------------------------------------------------------

fn handle_profiles(app: &mut App, code: KeyCode) {
    if try_switch_tab(app, code) {
        return;
    }
    match code {
        KeyCode::Char('q') | KeyCode::Esc => app.should_quit = true,
        KeyCode::Char('j') | KeyCode::Down => app.move_down(),
        KeyCode::Char('k') | KeyCode::Up => app.move_up(),
        KeyCode::Char('a') => {
            app.mode = Mode::AddProfile {
                focus: 0,
                name: String::new(),
                access_key: String::new(),
                secret_key: String::new(),
                region: String::new(),
            };
        }
        KeyCode::Char('e') => {
            if let Some(profile) = app.selected_profile() {
                let profile = profile.to_string();
                app.loading = Some("Loading profile...".into());
                app.spawn_load_profile_detail(&profile);
            }
        }
        KeyCode::Char('t') => {
            if let Some(profile) = app.selected_profile() {
                let profile = profile.to_string();
                app.profile_status.insert(profile.clone(), "testing".into());
                app.flash(format!("Testing '{}'...", profile));
                app.spawn_test_profile(&profile);
            }
        }
        KeyCode::Char('l') => {
            let profile = app.selected_profile().unwrap_or("").to_string();
            app.mode = Mode::LinkBucket {
                focus: 0,
                bucket: String::new(),
                region: String::new(),
                profile,
            };
        }
        KeyCode::Char('.') => app.submenu = true,
        _ => {}
    }
}

fn handle_profiles_submenu(app: &mut App, code: KeyCode) {
    match code {
        KeyCode::Char('.') | KeyCode::Esc => app.submenu = false,
        KeyCode::Char('S') => {
            app.submenu = false;
            app.flash("SSO: run `aws configure sso` in your terminal".into());
        }
        KeyCode::Char('u') => {
            // Unlink bucket from profile
            if let Some(profile) = app.selected_profile() {
                let profile = profile.to_string();
                let bucket_name = app
                    .config
                    .buckets
                    .iter()
                    .find(|b| {
                        let p = if b.profile.is_empty() {
                            "default"
                        } else {
                            &b.profile
                        };
                        p == profile
                    })
                    .map(|b| b.name.clone());

                if let Some(name) = bucket_name {
                    app.config.remove_bucket(&name);
                    let _ = app.config.save();
                    app.flash(format!("Unlinked '{}'", name));
                } else {
                    app.flash(format!("No buckets linked to '{}'", profile));
                }
                app.submenu = false;
            }
        }
        KeyCode::Char('d') => {
            if let Some(profile) = app.selected_profile() {
                let profile = profile.to_string();
                app.mode = Mode::Confirm {
                    action: ConfirmAction::DeleteProfile,
                    target: profile,
                };
                app.submenu = false;
            }
        }
        KeyCode::Char('D') => {
            if let Some(profile) = app.selected_profile() {
                let profile = profile.to_string();
                for b in &app.config.buckets {
                    let p = if b.profile.is_empty() {
                        "default"
                    } else {
                        &b.profile
                    };
                    if p == profile {
                        let name = b.name.clone();
                        app.config.set_default(&name);
                        let _ = app.config.save();
                        app.flash(format!("Default set to '{}'", name));
                        app.submenu = false;
                        return;
                    }
                }
                app.flash(format!("No buckets linked to '{}'", profile));
                app.submenu = false;
            }
        }
        KeyCode::Char('R') => {
            app.submenu = false;
            app.loading = Some("Loading profiles...".into());
            app.spawn_load_profiles();
        }
        KeyCode::Char('?') => {
            app.mode = Mode::Help;
            app.submenu = false;
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Restores handlers
// ---------------------------------------------------------------------------

fn handle_restores(app: &mut App, code: KeyCode) {
    if try_switch_tab(app, code) {
        return;
    }
    match code {
        KeyCode::Char('q') | KeyCode::Esc => app.should_quit = true,
        KeyCode::Char('j') | KeyCode::Down => app.move_down(),
        KeyCode::Char('k') | KeyCode::Up => app.move_up(),
        KeyCode::Char('c') => {
            if let Some(req) = app.selected_restore().cloned()
                && req.status == "pending"
            {
                app.loading = Some(format!("Checking {}...", req.id));
                app.spawn_check_restore(&req);
            }
        }
        KeyCode::Enter => {
            if let Some(req) = app.selected_restore().cloned()
                && req.status == "available"
            {
                app.mode = Mode::Confirm {
                    action: ConfirmAction::Download,
                    target: req.key,
                };
            }
        }
        KeyCode::Char('.') => app.submenu = true,
        _ => {}
    }
}

fn handle_restores_submenu(app: &mut App, code: KeyCode) {
    match code {
        KeyCode::Char('.') | KeyCode::Esc => app.submenu = false,
        KeyCode::Char('x') => {
            app.submenu = false;
            if let Some(req) = app.selected_restore().cloned()
                && req.status == "pending"
            {
                app.mode = Mode::Confirm {
                    action: ConfirmAction::CancelRestore,
                    target: req.id,
                };
            }
        }
        KeyCode::Char('D') => {
            app.submenu = false;
            if daemon::is_running() {
                match daemon::stop() {
                    Ok(()) => app.flash("Daemon stopped".into()),
                    Err(e) => app.flash(format!("Stop failed: {}", e)),
                }
            } else {
                match daemon::start() {
                    Ok(()) => app.flash("Daemon started".into()),
                    Err(e) => app.flash(format!("Start failed: {}", e)),
                }
            }
        }
        KeyCode::Char('R') => {
            app.submenu = false;
            let pending = app
                .restores
                .iter()
                .filter(|r| r.status == "pending")
                .count();
            if pending > 0 {
                app.loading = Some(format!("Checking {} pending restores...", pending));
                app.spawn_check_all_restores();
            } else {
                app.loading = Some("Refreshing restores...".into());
                app.spawn_load_restores();
            }
        }
        KeyCode::Char('d') => {
            app.submenu = false;
            if let Some(req) = app.selected_restore().cloned()
                && req.status != "pending"
            {
                app.mode = Mode::Confirm {
                    action: ConfirmAction::DeleteRestore,
                    target: req.id,
                };
            }
        }
        KeyCode::Char('?') => {
            app.mode = Mode::Help;
            app.submenu = false;
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Library handlers
// ---------------------------------------------------------------------------

fn handle_library(app: &mut App, code: KeyCode) {
    if try_switch_tab(app, code) {
        return;
    }
    match code {
        KeyCode::Char('q') | KeyCode::Esc => app.should_quit = true,
        KeyCode::Char('j') | KeyCode::Down => app.move_down(),
        KeyCode::Char('k') | KeyCode::Up => app.move_up(),
        KeyCode::Enter | KeyCode::Char('o') => {
            if let Some(file) = app.selected_cached_file().cloned() {
                let _ = std::process::Command::new("open").arg(&file.path).spawn();
                app.flash(format!("Opened {}", file.name));
            }
        }
        KeyCode::Char('d') => {
            if let Some(file) = app.selected_cached_file().cloned() {
                app.mode = Mode::Confirm {
                    action: ConfirmAction::DeleteCached,
                    target: format!("{}\0{}", file.bucket, file.key),
                };
            }
        }
        KeyCode::Char('r') => {
            if let Some(file) = app.selected_cached_file().cloned() {
                app.mode = Mode::Rename {
                    bucket: file.bucket.clone(),
                    key: file.key.clone(),
                    new_name: file.name.clone(),
                };
            }
        }
        KeyCode::Char('s') => {
            if let Some(file) = app.selected_cached_file().cloned() {
                match cache::copy_path_to_clipboard(&file.path) {
                    Ok(()) => app.flash("Path copied to clipboard".into()),
                    Err(e) => app.flash(format!("Copy failed: {}", e)),
                }
            }
        }
        KeyCode::Char('i') => {
            if let Some(file) = app.selected_cached_file().cloned() {
                app.mode = Mode::LibraryInfo { file };
            }
        }
        KeyCode::Char('p') => {
            if let Some(file) = app.selected_cached_file().cloned() {
                if is_image(&file.name) {
                    if app.load_image_preview_from_path(&file.path) {
                        app.mode = Mode::ImagePreview {
                            key: file.key.clone(),
                        };
                    } else {
                        app.flash("Cannot preview this image".into());
                    }
                } else {
                    app.flash("Not an image file".into());
                }
            }
        }
        KeyCode::Char('/') => {
            app.mode = Mode::Filter;
            app.library_filter.clear();
        }
        KeyCode::Char('.') => app.submenu = true,
        _ => {}
    }
}

fn handle_library_submenu(app: &mut App, code: KeyCode) {
    match code {
        KeyCode::Char('.') | KeyCode::Esc => app.submenu = false,
        KeyCode::Char('R') => {
            app.submenu = false;
            app.refresh_library();
            app.flash(format!("{} cached files", app.cached_files.len()));
        }
        KeyCode::Char('?') => {
            app.mode = Mode::Help;
            app.submenu = false;
        }
        _ => {}
    }
}

fn handle_rename(app: &mut App, code: KeyCode) {
    let Mode::Rename {
        bucket,
        key,
        new_name,
    } = &mut app.mode
    else {
        return;
    };
    match code {
        KeyCode::Esc => {
            app.mode = Mode::Normal;
        }
        KeyCode::Enter => {
            let b = bucket.clone();
            let k = key.clone();
            let n = new_name.clone();
            if n.is_empty() {
                return;
            }
            app.mode = Mode::Normal;
            match cache::rename_cached(&b, &k, &n) {
                Ok(_new_key) => {
                    app.refresh_library();
                    app.flash(format!("Renamed to {}", n));
                }
                Err(e) => {
                    app.flash(format!("Rename failed: {}", e));
                }
            }
        }
        KeyCode::Backspace => {
            new_name.pop();
        }
        KeyCode::Char(c) => {
            new_name.push(c);
        }
        _ => {}
    }
}

fn handle_library_info(app: &mut App, code: KeyCode) {
    match code {
        KeyCode::Esc | KeyCode::Char('q') => {
            app.mode = Mode::Normal;
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Tier picker
// ---------------------------------------------------------------------------

fn handle_tier_picker(app: &mut App, code: KeyCode) {
    let Mode::TierPicker {
        key,
        storage_class,
        selected,
    } = &mut app.mode
    else {
        return;
    };
    match code {
        KeyCode::Esc | KeyCode::Char('q') => {
            app.mode = Mode::Normal;
        }
        KeyCode::Char('j') | KeyCode::Down => {
            if *selected < TIER_OPTIONS.len() - 1 {
                *selected += 1;
            }
        }
        KeyCode::Char('k') | KeyCode::Up => {
            if *selected > 0 {
                *selected -= 1;
            }
        }
        KeyCode::Enter => {
            let tier = TIER_OPTIONS[*selected];
            let key = key.clone();
            let sc = storage_class.clone();
            if tier == "Expedited" && sc == "DEEP_ARCHIVE" {
                app.mode = Mode::Normal;
                app.flash("Expedited not available for DEEP_ARCHIVE".into());
                return;
            }
            app.mode = Mode::Normal;
            let name = key.rsplit('/').next().unwrap_or(&key);
            app.loading = Some(format!("Restoring {}...", name));
            app.spawn_submit_restore(&key, tier, &sc);
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Image preview
// ---------------------------------------------------------------------------

fn handle_image_preview(app: &mut App, code: KeyCode) {
    let Mode::ImagePreview { key } = &app.mode else {
        return;
    };
    match code {
        KeyCode::Esc | KeyCode::Char('q') => {
            app.image_protocol = None;
            app.mode = Mode::Normal;
        }
        KeyCode::Char('o') => {
            let key = key.clone();
            app.image_protocol = None;
            app.mode = Mode::Normal;
            let name = key.rsplit('/').next().unwrap_or(&key).to_string();
            if cache::is_cached(&app.bucket, &key) {
                let _ = cache::open_cached(&app.bucket, &key);
                app.flash(format!("Opened {} (cached)", name));
            } else {
                app.loading = Some(format!("Caching {}...", name));
                app.spawn_cache_and_open(&key);
            }
        }
        KeyCode::Char('s') => {
            let key = key.clone();
            app.image_protocol = None;
            app.mode = Mode::Normal;
            app.loading = Some("Loading metadata...".into());
            app.spawn_head_object(&key);
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Bucket picker
// ---------------------------------------------------------------------------

fn handle_bucket_picker(app: &mut App, code: KeyCode) {
    let Mode::BucketPicker { buckets, selected } = &mut app.mode else {
        return;
    };
    match code {
        KeyCode::Esc | KeyCode::Char('q') => {
            if !app.bucket.is_empty() {
                app.mode = Mode::Normal;
            }
        }
        KeyCode::Char('j') | KeyCode::Down => {
            if *selected < buckets.len().saturating_sub(1) {
                *selected += 1;
            }
        }
        KeyCode::Char('k') | KeyCode::Up => {
            if *selected > 0 {
                *selected -= 1;
            }
        }
        KeyCode::Enter => {
            if let Some(bucket) = buckets.get(*selected) {
                app.bucket = bucket.clone();
                app.prefix.clear();
                app.mode = Mode::Normal;
                app.awaiting_buckets = false;
                app.loading = Some("Loading...".into());
                app.spawn_list_objects();
            }
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Confirm dialog
// ---------------------------------------------------------------------------

fn handle_confirm(app: &mut App, code: KeyCode) {
    let Mode::Confirm { action, target } = &app.mode else {
        return;
    };
    match code {
        KeyCode::Char('y') | KeyCode::Char('Y') => {
            let action = action.clone();
            let target = target.clone();
            app.mode = Mode::Normal;
            match action {
                ConfirmAction::Download => {
                    let name = target.rsplit('/').next().unwrap_or(&target);
                    app.loading = Some(format!("Downloading {}...", name));
                    app.spawn_download(&target);
                }
                ConfirmAction::DeleteProfile => {
                    app.loading = Some(format!("Deleting '{}'...", target));
                    app.spawn_delete_profile(&target);
                }
                ConfirmAction::CancelRestore => {
                    let _ = restore::update_status(&target, "cancelled", None);
                    app.restores = restore::list_requests();
                    app.flash(format!("Cancelled {}", target));
                }
                ConfirmAction::DeleteRestore => {
                    let _ = restore::remove_request(&target);
                    app.restores = restore::list_requests();
                    app.clamp_restore_selection();
                    app.flash("Removed completed restore".into());
                }
                ConfirmAction::DeleteCached => {
                    // target is "bucket\0key"
                    if let Some((bucket, key)) = target.split_once('\0') {
                        let _ = cache::delete_cached(bucket, key);
                        app.refresh_library();
                        app.flash("Deleted cached file".into());
                    }
                }
            }
        }
        KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
            app.mode = Mode::Normal;
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Filter
// ---------------------------------------------------------------------------

fn handle_filter(app: &mut App, code: KeyCode) {
    let is_library = app.tab == Tab::Library;
    match code {
        KeyCode::Enter | KeyCode::Esc => {
            app.mode = Mode::Normal;
            if is_library {
                app.library_selected = 0;
            } else {
                app.browse_selected = 0;
            }
        }
        KeyCode::Backspace => {
            if is_library {
                app.library_filter.pop();
                app.library_selected = 0;
            } else {
                app.filter_text.pop();
                app.browse_selected = 0;
            }
        }
        KeyCode::Char(c) => {
            if is_library {
                app.library_filter.push(c);
                app.library_selected = 0;
            } else {
                app.filter_text.push(c);
                app.browse_selected = 0;
            }
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Detail view
// ---------------------------------------------------------------------------

fn handle_detail(app: &mut App, code: KeyCode) {
    let Mode::Detail { info } = &app.mode else {
        return;
    };
    match code {
        KeyCode::Esc | KeyCode::Char('q') => {
            app.mode = Mode::Normal;
        }
        KeyCode::Char('g') => {
            let key = info.key.clone();
            let class = info.storage_class.clone();
            let restore = info.restore_status.clone();
            app.mode = Mode::Normal;
            if is_glacier(&class) && restore != "available" {
                app.flash("Object is in Glacier — restore it first (r)".into());
            } else {
                app.mode = Mode::Confirm {
                    action: ConfirmAction::Download,
                    target: key,
                };
            }
        }
        KeyCode::Char('o') => {
            let key = info.key.clone();
            let class = info.storage_class.clone();
            let restore = info.restore_status.clone();
            app.mode = Mode::Normal;
            if is_glacier(&class) && restore != "available" {
                app.flash("Object is in Glacier — restore it first (r)".into());
            } else {
                let name = key.rsplit('/').next().unwrap_or(&key).to_string();
                if cache::is_cached(&app.bucket, &key) {
                    let _ = cache::open_cached(&app.bucket, &key);
                    app.flash(format!("Opened {} (cached)", name));
                } else {
                    app.loading = Some(format!("Caching {}...", name));
                    app.spawn_cache_and_open(&key);
                }
            }
        }
        KeyCode::Char('r') => {
            let key = info.key.clone();
            let class = info.storage_class.clone();
            if is_glacier(&class) {
                app.mode = Mode::TierPicker {
                    key,
                    storage_class: class,
                    selected: 0,
                };
            }
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Link bucket form
// ---------------------------------------------------------------------------

fn handle_link_form(app: &mut App, code: KeyCode) {
    let Mode::LinkBucket {
        focus,
        bucket,
        region,
        profile,
    } = &mut app.mode
    else {
        return;
    };
    match code {
        KeyCode::Esc => app.mode = Mode::Normal,
        KeyCode::Tab => *focus = (*focus + 1) % 3,
        KeyCode::BackTab => *focus = (*focus + 2) % 3,
        KeyCode::Enter => {
            if !bucket.is_empty() {
                let b = bucket.clone();
                let r = region.clone();
                let p = profile.clone();
                app.config.add_bucket(&b, &r, &p);
                let _ = app.config.save();
                app.flash(format!("Linked '{}' → '{}'", b, p));
                app.mode = Mode::Normal;
            }
        }
        KeyCode::Backspace => {
            let field = match focus {
                0 => bucket,
                1 => region,
                _ => profile,
            };
            field.pop();
        }
        KeyCode::Char(c) => {
            let field = match focus {
                0 => bucket,
                1 => region,
                _ => profile,
            };
            field.push(c);
        }
        _ => {}
    }
}

// ---------------------------------------------------------------------------
// Upload form
// ---------------------------------------------------------------------------

/// List filesystem entries matching a partial path for tab completion.
fn complete_path(partial: &str) -> Vec<String> {
    use std::path::Path;

    let path = Path::new(partial);
    let (dir, prefix) = if partial.ends_with('/') || partial.ends_with(std::path::MAIN_SEPARATOR) {
        (path.to_path_buf(), "")
    } else {
        let dir = path.parent().unwrap_or(Path::new("."));
        let prefix = path
            .file_name()
            .map(|f| f.to_str().unwrap_or(""))
            .unwrap_or("");
        (dir.to_path_buf(), prefix)
    };

    let entries = match std::fs::read_dir(&dir) {
        Ok(e) => e,
        Err(_) => return Vec::new(),
    };

    let mut matches: Vec<(String, bool)> = entries
        .filter_map(|e| {
            let e = e.ok()?;
            let name = e.file_name().to_string_lossy().to_string();
            // Skip hidden files unless the user is explicitly typing a dot prefix
            if name.starts_with('.') && !prefix.starts_with('.') {
                return None;
            }
            if !name.starts_with(prefix) {
                return None;
            }
            let is_dir = e.file_type().ok()?.is_dir();
            let full = if dir == Path::new(".") && !partial.starts_with("./") {
                name.clone()
            } else {
                dir.join(&name).to_string_lossy().to_string()
            };
            let display = if is_dir { format!("{}/", full) } else { full };
            Some((display, is_dir))
        })
        .collect();

    // Directories first, then files, alphabetical within each group
    matches.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
    matches.into_iter().map(|(s, _)| s).collect()
}

fn handle_upload_form(app: &mut App, code: KeyCode) {
    let Mode::Upload {
        focus,
        local_path,
        key,
        storage_class,
        completions,
        comp_selected,
    } = &mut app.mode
    else {
        return;
    };

    match code {
        KeyCode::Esc => {
            if !completions.is_empty() {
                // First Esc dismisses completions
                completions.clear();
                *comp_selected = None;
            } else {
                app.mode = Mode::Normal;
            }
        }
        KeyCode::Tab if *focus == 0 => {
            // Tab completion for file path
            if completions.is_empty() {
                // Generate completions
                let matches = complete_path(local_path);
                if matches.len() == 1 {
                    // Single match — complete it directly
                    *local_path = matches[0].clone();
                } else if !matches.is_empty() {
                    // Fill common prefix
                    let common = longest_common_prefix(&matches);
                    if common.len() > local_path.len() {
                        *local_path = common;
                    }
                    *completions = matches;
                    *comp_selected = Some(0);
                }
            } else {
                // Cycle through completions
                let sel = comp_selected.get_or_insert(0);
                *sel = (*sel + 1) % completions.len();
                *local_path = completions[*sel].clone();
            }
        }
        KeyCode::BackTab if *focus == 0 && !completions.is_empty() => {
            // Cycle backwards through completions
            let sel = comp_selected.get_or_insert(0);
            *sel = if *sel == 0 {
                completions.len() - 1
            } else {
                *sel - 1
            };
            *local_path = completions[*sel].clone();
        }
        KeyCode::Tab => {
            // Move to next field (non-file fields)
            if *focus == 0 && !local_path.is_empty() {
                let filename = std::path::Path::new(local_path.as_str())
                    .file_name()
                    .unwrap_or_default()
                    .to_string_lossy();
                if key.is_empty() || key.ends_with('/') {
                    key.push_str(&filename);
                }
            }
            completions.clear();
            *comp_selected = None;
            *focus = (*focus + 1) % 3;
        }
        KeyCode::BackTab => {
            completions.clear();
            *comp_selected = None;
            *focus = (*focus + 2) % 3;
        }
        KeyCode::Enter => {
            if *focus == 0 && !completions.is_empty() {
                // Accept the selected completion
                if let Some(sel) = comp_selected {
                    *local_path = completions[*sel].clone();
                }
                completions.clear();
                *comp_selected = None;
                // If it's a directory, stay on this field for further completion
                if local_path.ends_with('/') {
                    return;
                }
                return;
            }
            let lp = local_path.clone();
            let k = key.clone();
            let sc = storage_class.clone();
            if lp.is_empty() {
                app.flash("File path is required".into());
                return;
            }
            if !std::path::Path::new(&lp).exists() {
                app.flash(format!("File not found: {}", lp));
                return;
            }
            if std::path::Path::new(&lp).is_dir() {
                app.flash("Path is a directory, not a file".into());
                return;
            }
            let final_key = if k.is_empty() || k.ends_with('/') {
                let filename = std::path::Path::new(&lp)
                    .file_name()
                    .unwrap_or_default()
                    .to_string_lossy();
                format!("{}{}", k, filename)
            } else {
                k
            };
            let name = final_key.rsplit('/').next().unwrap_or(&final_key);
            app.loading = Some(format!("Uploading {}...", name));
            app.mode = Mode::Normal;
            app.spawn_upload(&lp, &final_key, &sc);
        }
        KeyCode::Backspace => {
            let field = match focus {
                0 => {
                    completions.clear();
                    *comp_selected = None;
                    local_path
                }
                1 => key,
                _ => storage_class,
            };
            field.pop();
        }
        KeyCode::Char(c) => {
            let field = match focus {
                0 => {
                    completions.clear();
                    *comp_selected = None;
                    local_path
                }
                1 => key,
                _ => storage_class,
            };
            field.push(c);
        }
        _ => {}
    }
}

fn longest_common_prefix(strings: &[String]) -> String {
    if strings.is_empty() {
        return String::new();
    }
    let first = &strings[0];
    let mut len = first.len();
    for s in &strings[1..] {
        len = len.min(s.len());
        for (i, (a, b)) in first.bytes().zip(s.bytes()).enumerate() {
            if a != b {
                len = len.min(i);
                break;
            }
        }
    }
    first[..len].to_string()
}

// ---------------------------------------------------------------------------
// Profile form (add/edit)
// ---------------------------------------------------------------------------

fn handle_profile_form(app: &mut App, code: KeyCode) {
    let (focus, name, access_key, secret_key, region, is_edit) = match &mut app.mode {
        Mode::AddProfile {
            focus,
            name,
            access_key,
            secret_key,
            region,
        } => (focus, name, access_key, secret_key, region, false),
        Mode::EditProfile {
            focus,
            name,
            access_key,
            secret_key,
            region,
        } => (focus, name, access_key, secret_key, region, true),
        _ => return,
    };

    match code {
        KeyCode::Esc => app.mode = Mode::Normal,
        KeyCode::Tab => {
            *focus = (*focus + 1) % 4;
            if is_edit && *focus == 0 {
                *focus = 1; // skip name when editing
            }
        }
        KeyCode::BackTab => {
            *focus = (*focus + 3) % 4;
            if is_edit && *focus == 0 {
                *focus = 3;
            }
        }
        KeyCode::Enter => {
            if name.is_empty() {
                return; // need a name
            }
            if access_key.is_empty() {
                return; // need access key
            }
            if !is_edit && secret_key.is_empty() {
                return; // need secret key for new profiles
            }
            let n = name.clone();
            let ak = access_key.clone();
            let sk = secret_key.clone();
            let r = region.clone();
            app.mode = Mode::Normal;
            app.loading = Some(format!("Saving '{}'...", n));
            app.spawn_save_profile(&n, &ak, &sk, &r);
        }
        KeyCode::Backspace => {
            let field = match focus {
                0 => name,
                1 => access_key,
                2 => secret_key,
                _ => region,
            };
            field.pop();
        }
        KeyCode::Char(c) => {
            let field = match focus {
                0 => name,
                1 => access_key,
                2 => secret_key,
                _ => region,
            };
            field.push(c);
        }
        _ => {}
    }
}
