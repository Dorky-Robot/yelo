use std::collections::HashMap;
use std::sync::mpsc;
use std::time::Instant;

use ratatui_image::picker::Picker;
use ratatui_image::protocol::StatefulProtocol;

use crate::aws_ops::ObjectInfo;
use crate::cache;
use crate::config::Config;
use crate::credentials;
use crate::log;
use crate::restore::{self, RestoreNotification};
use crate::state::State;
use crate::{aws_ops, credentials as creds};

pub const SPINNER: &[&str] = &["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Tab {
    Browse,
    Profiles,
    Restores,
    Library,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ConfirmAction {
    Download,
    DeleteProfile,
    CancelRestore,
    DeleteRestore,
    DeleteCached,
}

#[derive(Debug, Clone)]
pub enum Mode {
    Normal,
    Help,
    BucketPicker {
        buckets: Vec<String>,
        selected: usize,
    },
    Confirm {
        action: ConfirmAction,
        target: String,
    },
    Filter,
    Detail {
        info: ObjectInfo,
    },
    LinkBucket {
        focus: usize,
        bucket: String,
        region: String,
        profile: String,
    },
    AddProfile {
        focus: usize,
        name: String,
        access_key: String,
        secret_key: String,
        region: String,
    },
    EditProfile {
        focus: usize,
        name: String,
        access_key: String,
        secret_key: String,
        region: String,
    },
    TierPicker {
        key: String,
        storage_class: String,
        selected: usize,
    },
    ImagePreview {
        key: String,
    },
    Rename {
        bucket: String,
        key: String,
        new_name: String,
    },
    LibraryInfo {
        file: cache::CachedFile,
    },
    Upload {
        focus: usize,
        local_path: String,
        key: String,
        storage_class: String,
        completions: Vec<String>,
        comp_selected: Option<usize>,
    },
}

pub enum BgResult {
    ListObjects {
        items: Vec<ObjectInfo>,
        err: Option<String>,
    },
    ObjectDetail {
        info: Option<ObjectInfo>,
        err: Option<String>,
    },
    BucketList {
        buckets: Vec<String>,
        err: Option<String>,
    },
    DownloadComplete {
        key: String,
        local_path: String,
        err: Option<String>,
    },
    ProfileList {
        profiles: Vec<String>,
        err: Option<String>,
    },
    ProfileTest {
        profile: String,
        ok: bool,
        err: Option<String>,
    },
    ProfileSaved {
        profile: String,
        err: Option<String>,
    },
    ProfileDeleted {
        profile: String,
        err: Option<String>,
    },
    ProfileDetail {
        profile: String,
        access_key: String,
        region: String,
    },
    RestoreSubmitted {
        id: String,
        err: Option<String>,
    },
    RestoreChecked {
        id: String,
        new_status: String,
        err: Option<String>,
    },
    RestoresLoaded {
        restores: Vec<RestoreNotification>,
    },
    CacheComplete {
        key: String,
        err: Option<String>,
    },
    CacheForPreview {
        key: String,
        err: Option<String>,
    },
    UploadComplete {
        key: String,
        err: Option<String>,
    },
}

pub struct App {
    pub config: Config,
    pub state: State,
    pub tab: Tab,
    pub mode: Mode,
    pub submenu: bool,
    pub should_quit: bool,

    // Browse
    pub bucket: String,
    pub prefix: String,
    pub items: Vec<ObjectInfo>,
    pub browse_selected: usize,
    pub browse_scroll: usize,
    pub filter_text: String,

    // Profiles
    pub profiles: Vec<String>,
    pub profile_selected: usize,
    pub profile_status: HashMap<String, String>,

    // Restores
    pub restores: Vec<RestoreNotification>,
    pub restore_selected: usize,
    pub restore_scroll: usize,

    // Library
    pub cached_files: Vec<cache::CachedFile>,
    pub library_selected: usize,
    pub library_scroll: usize,
    pub library_filter: String,

    // Image preview
    pub image_protocol: Option<StatefulProtocol>,
    pub picker: Picker,

    // UI
    pub loading: Option<String>,
    pub status_msg: Option<String>,
    pub status_expires: Option<Instant>,
    pub spinner_tick: usize,

    pub awaiting_buckets: bool,
    bg_tx: mpsc::Sender<BgResult>,
    bg_rx: mpsc::Receiver<BgResult>,
}

impl App {
    pub fn new(picker: Picker) -> Self {
        let config = Config::load().unwrap_or_default();
        let state = State::load().unwrap_or_default();
        let (tx, rx) = mpsc::channel();

        let bucket = if !state.bucket.is_empty() {
            state.bucket.clone()
        } else {
            config.resolve_bucket().unwrap_or_default()
        };

        let mut app = App {
            config,
            state,
            tab: Tab::Browse,
            mode: Mode::Normal,
            submenu: false,
            should_quit: false,
            bucket: bucket.clone(),
            prefix: String::new(),
            items: Vec::new(),
            browse_selected: 0,
            browse_scroll: 0,
            filter_text: String::new(),
            profiles: Vec::new(),
            profile_selected: 0,
            profile_status: HashMap::new(),
            restores: Vec::new(),
            restore_selected: 0,
            restore_scroll: 0,
            cached_files: Vec::new(),
            library_selected: 0,
            library_scroll: 0,
            library_filter: String::new(),
            image_protocol: None,
            picker,
            loading: None,
            status_msg: None,
            status_expires: None,
            spinner_tick: 0,
            awaiting_buckets: false,
            bg_tx: tx,
            bg_rx: rx,
        };

        if !app.state.prefix.is_empty() {
            app.prefix = app.state.prefix.clone();
        }

        log::log(&format!(
            "App::new bucket={:?} prefix={:?} config.region={:?} config.profile={:?}",
            app.bucket, app.prefix, app.config.region, app.config.profile
        ));

        // Kick off initial load
        if bucket.is_empty() {
            log::log("No bucket set, loading bucket list");
            app.loading = Some("Loading buckets...".into());
            app.spawn_list_buckets();
        } else {
            log::log(&format!("Loading objects for bucket {:?}", bucket));
            app.loading = Some("Loading...".into());
            app.spawn_list_objects();
        }

        app
    }

    pub fn poll_bg(&mut self) {
        // Auto-clear expired status messages
        if let Some(expires) = self.status_expires
            && Instant::now() >= expires
        {
            self.status_msg = None;
            self.status_expires = None;
        }

        while let Ok(result) = self.bg_rx.try_recv() {
            self.loading = None;
            match result {
                BgResult::ListObjects { items, err } => {
                    if let Some(e) = err {
                        self.flash_sticky(format!("Error: {} — press .b to pick a bucket", e));
                        self.loading = Some("Loading buckets...".into());
                        self.spawn_list_buckets();
                    } else {
                        self.items = items;
                        self.browse_selected = 0;
                        self.browse_scroll = 0;
                        self.flash(format!("{} items", self.items.len()));
                    }
                }
                BgResult::ObjectDetail { info, err } => {
                    if let Some(e) = err {
                        self.flash(format!("Error: {}", e));
                    } else if let Some(info) = info {
                        self.mode = Mode::Detail { info };
                    }
                }
                BgResult::BucketList { buckets, err } => {
                    if !self.awaiting_buckets {
                        // Stale result from a previous request — ignore
                        log::log("BgResult::BucketList ignored (not awaiting)");
                    } else {
                        self.awaiting_buckets = false;
                        if let Some(e) = err {
                            log::log(&format!("BgResult::BucketList error: {}", e));
                            self.flash_sticky(format!("Error loading buckets: {}", e));
                        } else if buckets.is_empty() {
                            self.flash_sticky(
                                "No buckets found — check your AWS credentials".into(),
                            );
                        } else {
                            self.mode = Mode::BucketPicker {
                                buckets,
                                selected: 0,
                            };
                        }
                    }
                }
                BgResult::DownloadComplete {
                    key,
                    local_path,
                    err,
                } => {
                    if let Some(e) = err {
                        self.flash(format!("Download failed: {}", e));
                    } else {
                        let name = key.rsplit('/').next().unwrap_or(&key);
                        self.flash(format!("Downloaded {} → {}", name, local_path));
                    }
                }
                BgResult::ProfileList { profiles, err } => {
                    if let Some(e) = err {
                        self.flash(format!("Error: {}", e));
                    } else {
                        self.profiles = profiles;
                        self.clamp_profile_selection();
                    }
                }
                BgResult::ProfileTest { profile, ok, err } => {
                    if ok {
                        self.profile_status.insert(profile.clone(), "ok".into());
                        self.flash(format!("'{}' connected", profile));
                    } else {
                        self.profile_status.insert(profile.clone(), "fail".into());
                        let msg = err.unwrap_or_else(|| "unknown error".into());
                        self.flash(format!("'{}' failed: {}", profile, msg));
                    }
                }
                BgResult::ProfileSaved { profile, err } => {
                    if let Some(e) = err {
                        self.flash(format!("Error: {}", e));
                    } else {
                        self.flash(format!("Profile '{}' saved", profile));
                        self.spawn_load_profiles();
                    }
                }
                BgResult::ProfileDeleted { profile, err } => {
                    if let Some(e) = err {
                        self.flash(format!("Error: {}", e));
                    } else {
                        self.flash(format!("Profile '{}' deleted", profile));
                        self.spawn_load_profiles();
                    }
                }
                BgResult::ProfileDetail {
                    profile,
                    access_key,
                    region,
                } => {
                    self.mode = Mode::EditProfile {
                        focus: 1,
                        name: profile,
                        access_key,
                        secret_key: String::new(),
                        region,
                    };
                }
                BgResult::RestoreSubmitted { id, err } => {
                    if let Some(e) = err {
                        self.flash(format!("Restore failed: {}", e));
                    } else {
                        self.flash(format!("Restore requested — tracking as {}", id));
                        // Reload restores list
                        self.restores = restore::list_requests();
                    }
                }
                BgResult::RestoreChecked {
                    id,
                    new_status,
                    err,
                } => {
                    if let Some(e) = err {
                        self.flash(format!("Check failed: {}", e));
                    } else {
                        let _ = restore::update_status(&id, &new_status, None);
                        self.restores = restore::list_requests();
                        self.flash(format!("{}: {}", id, new_status));
                    }
                }
                BgResult::RestoresLoaded { restores } => {
                    self.restores = restores;
                    self.clamp_restore_selection();
                }
                BgResult::CacheComplete { key, err } => {
                    if let Some(e) = err {
                        self.flash(format!("Cache failed: {}", e));
                    } else {
                        let name = key.rsplit('/').next().unwrap_or(&key);
                        let _ = cache::open_cached(&self.bucket, &key);
                        self.flash(format!("Opened {}", name));
                    }
                }
                BgResult::CacheForPreview { key, err } => {
                    if let Some(e) = err {
                        log::log(&format!("CacheForPreview error: {}", e));
                        self.flash(format!("Cache failed: {}", e));
                    } else {
                        log::log(&format!(
                            "CacheForPreview complete, loading preview for {:?}",
                            key
                        ));
                        if self.load_image_preview(&key) {
                            self.mode = Mode::ImagePreview { key };
                        } else {
                            // Preview failed, fall back to detail
                            self.loading = Some("Loading metadata...".into());
                            self.spawn_head_object(&key);
                        }
                    }
                }
                BgResult::UploadComplete { key, err } => {
                    if let Some(e) = err {
                        self.flash(format!("Upload failed: {}", e));
                    } else {
                        let name = key.rsplit('/').next().unwrap_or(&key);
                        self.flash(format!("Uploaded {}", name));
                        self.loading = Some("Refreshing...".into());
                        self.spawn_list_objects();
                    }
                }
            }
        }
    }

    pub fn flash(&mut self, msg: String) {
        self.status_msg = Some(msg);
        self.status_expires = Some(Instant::now() + std::time::Duration::from_secs(5));
    }

    pub fn flash_sticky(&mut self, msg: String) {
        self.status_msg = Some(msg);
        self.status_expires = None; // stays until next action
    }

    pub fn save_state(&mut self) {
        self.state.bucket = self.bucket.clone();
        self.state.prefix = self.prefix.clone();
        let _ = self.state.save();
    }

    // --- Navigation ---

    pub fn filtered_items(&self) -> Vec<&ObjectInfo> {
        if self.filter_text.is_empty() {
            self.items.iter().collect()
        } else {
            let lower = self.filter_text.to_lowercase();
            self.items
                .iter()
                .filter(|item| {
                    let name = display_name(&item.key, &self.prefix).to_lowercase();
                    name.contains(&lower)
                })
                .collect()
        }
    }

    pub fn move_up(&mut self) {
        match self.tab {
            Tab::Browse => {
                if self.browse_selected > 0 {
                    self.browse_selected -= 1;
                }
            }
            Tab::Profiles => {
                if self.profile_selected > 0 {
                    self.profile_selected -= 1;
                }
            }
            Tab::Restores => {
                if self.restore_selected > 0 {
                    self.restore_selected -= 1;
                }
            }
            Tab::Library => {
                if self.library_selected > 0 {
                    self.library_selected -= 1;
                }
            }
        }
    }

    pub fn move_down(&mut self) {
        match self.tab {
            Tab::Browse => {
                let max = self.filtered_items().len().saturating_sub(1);
                if self.browse_selected < max {
                    self.browse_selected += 1;
                }
            }
            Tab::Profiles => {
                let max = self.profiles.len().saturating_sub(1);
                if self.profile_selected < max {
                    self.profile_selected += 1;
                }
            }
            Tab::Restores => {
                let max = self.restores.len().saturating_sub(1);
                if self.restore_selected < max {
                    self.restore_selected += 1;
                }
            }
            Tab::Library => {
                let max = self.filtered_library_items().len().saturating_sub(1);
                if self.library_selected < max {
                    self.library_selected += 1;
                }
            }
        }
    }

    fn clamp_profile_selection(&mut self) {
        if !self.profiles.is_empty() && self.profile_selected >= self.profiles.len() {
            self.profile_selected = self.profiles.len() - 1;
        }
    }

    pub fn clamp_restore_selection(&mut self) {
        if !self.restores.is_empty() && self.restore_selected >= self.restores.len() {
            self.restore_selected = self.restores.len() - 1;
        }
    }

    pub fn selected_restore(&self) -> Option<&RestoreNotification> {
        self.restores.get(self.restore_selected)
    }

    pub fn selected_profile(&self) -> Option<&str> {
        self.profiles.get(self.profile_selected).map(|s| s.as_str())
    }

    // --- Background spawns ---

    pub fn spawn_list_objects(&self) {
        let tx = self.bg_tx.clone();
        let bucket = self.bucket.clone();
        let prefix = self.prefix.clone();
        let region = self.config.resolve_region(&bucket);
        let profile = self.config.resolve_profile(&bucket);
        std::thread::spawn(move || {
            let result = aws_ops::list_objects(&bucket, &prefix, &region, &profile);
            let _ = tx.send(match result {
                Ok(items) => BgResult::ListObjects { items, err: None },
                Err(e) => BgResult::ListObjects {
                    items: vec![],
                    err: Some(e.to_string()),
                },
            });
        });
    }

    pub fn spawn_head_object(&self, key: &str) {
        let tx = self.bg_tx.clone();
        let bucket = self.bucket.clone();
        let key = key.to_string();
        let region = self.config.resolve_region(&bucket);
        let profile = self.config.resolve_profile(&bucket);
        std::thread::spawn(move || {
            let result = aws_ops::head_object(&bucket, &key, &region, &profile);
            let _ = tx.send(match result {
                Ok(info) => BgResult::ObjectDetail {
                    info: Some(info),
                    err: None,
                },
                Err(e) => BgResult::ObjectDetail {
                    info: None,
                    err: Some(e.to_string()),
                },
            });
        });
    }

    pub fn spawn_list_buckets(&mut self) {
        self.awaiting_buckets = true;
        let tx = self.bg_tx.clone();
        let region = if self.config.region.is_empty() {
            "us-east-1".to_string()
        } else {
            self.config.region.clone()
        };
        let profile = self.config.profile.clone();
        std::thread::spawn(move || {
            let result = aws_ops::list_buckets(&region, &profile);
            let _ = tx.send(match result {
                Ok(buckets) => BgResult::BucketList { buckets, err: None },
                Err(e) => BgResult::BucketList {
                    buckets: vec![],
                    err: Some(e.to_string()),
                },
            });
        });
    }

    pub fn spawn_download(&self, key: &str) {
        let tx = self.bg_tx.clone();
        let bucket = self.bucket.clone();
        let key = key.to_string();
        let region = self.config.resolve_region(&bucket);
        let profile = self.config.resolve_profile(&bucket);
        std::thread::spawn(move || {
            let result = aws_ops::download(&bucket, &key, &region, &profile);
            let _ = tx.send(match result {
                Ok(local_path) => BgResult::DownloadComplete {
                    key,
                    local_path,
                    err: None,
                },
                Err(e) => BgResult::DownloadComplete {
                    key,
                    local_path: String::new(),
                    err: Some(e.to_string()),
                },
            });
        });
    }

    pub fn spawn_upload(&self, local_path: &str, key: &str, storage_class: &str) {
        let tx = self.bg_tx.clone();
        let bucket = self.bucket.clone();
        let key = key.to_string();
        let local = std::path::PathBuf::from(local_path);
        let sc = storage_class.to_string();
        let region = self.config.resolve_region(&bucket);
        let profile = self.config.resolve_profile(&bucket);
        std::thread::spawn(move || {
            let result = aws_ops::upload(&bucket, &key, &local, &sc, &region, &profile);
            let _ = tx.send(match result {
                Ok(()) => BgResult::UploadComplete { key, err: None },
                Err(e) => BgResult::UploadComplete {
                    key,
                    err: Some(e.to_string()),
                },
            });
        });
    }

    pub fn spawn_load_profiles(&self) {
        let tx = self.bg_tx.clone();
        std::thread::spawn(move || {
            let result = credentials::read_profiles();
            let _ = tx.send(match result {
                Ok(profiles) => BgResult::ProfileList {
                    profiles,
                    err: None,
                },
                Err(e) => BgResult::ProfileList {
                    profiles: vec![],
                    err: Some(e.to_string()),
                },
            });
        });
    }

    pub fn spawn_test_profile(&self, profile: &str) {
        let tx = self.bg_tx.clone();
        let profile = profile.to_string();
        let region = self.config.resolve_region("");
        std::thread::spawn(move || {
            let result = aws_ops::test_profile(&region, &profile);
            let _ = tx.send(BgResult::ProfileTest {
                profile,
                ok: result.is_ok(),
                err: result.err().map(|e| e.to_string()),
            });
        });
    }

    pub fn spawn_save_profile(&self, name: &str, access_key: &str, secret_key: &str, region: &str) {
        let tx = self.bg_tx.clone();
        let name = name.to_string();
        let ak = access_key.to_string();
        let sk = secret_key.to_string();
        let r = region.to_string();
        std::thread::spawn(move || {
            let result = creds::save_profile(&name, &ak, &sk, &r);
            let _ = tx.send(BgResult::ProfileSaved {
                profile: name,
                err: result.err().map(|e| e.to_string()),
            });
        });
    }

    pub fn spawn_delete_profile(&self, name: &str) {
        let tx = self.bg_tx.clone();
        let name = name.to_string();
        std::thread::spawn(move || {
            let result = creds::delete_profile(&name);
            let _ = tx.send(BgResult::ProfileDeleted {
                profile: name,
                err: result.err().map(|e| e.to_string()),
            });
        });
    }

    pub fn spawn_load_profile_detail(&self, name: &str) {
        let tx = self.bg_tx.clone();
        let name = name.to_string();
        std::thread::spawn(move || {
            let (access_key, region) = creds::read_profile_detail(&name);
            let _ = tx.send(BgResult::ProfileDetail {
                profile: name,
                access_key,
                region,
            });
        });
    }

    pub fn spawn_submit_restore(&self, key: &str, tier: &str, storage_class: &str) {
        let tx = self.bg_tx.clone();
        let bucket = self.bucket.clone();
        let key = key.to_string();
        let region = self.config.resolve_region(&bucket);
        let profile = self.config.resolve_profile(&bucket);
        let tier = tier.to_string();
        let storage_class = storage_class.to_string();
        std::thread::spawn(move || {
            let result =
                restore::submit_restore(&bucket, &key, &region, &profile, 7, &tier, &storage_class);
            let _ = tx.send(match result {
                Ok(req) => BgResult::RestoreSubmitted {
                    id: req.id,
                    err: None,
                },
                Err(e) => BgResult::RestoreSubmitted {
                    id: String::new(),
                    err: Some(e.to_string()),
                },
            });
        });
    }

    pub fn spawn_check_restore(&self, req: &RestoreNotification) {
        let tx = self.bg_tx.clone();
        let req = req.clone();
        std::thread::spawn(move || {
            let result = restore::check_restore(&req);
            let _ = tx.send(match result {
                Ok(new_status) => BgResult::RestoreChecked {
                    id: req.id,
                    new_status,
                    err: None,
                },
                Err(e) => BgResult::RestoreChecked {
                    id: req.id,
                    new_status: String::new(),
                    err: Some(e.to_string()),
                },
            });
        });
    }

    pub fn load_image_preview(&mut self, key: &str) -> bool {
        let path = cache::cached_path(&self.bucket, key);
        self.load_image_preview_from_path(&path)
    }

    pub fn load_image_preview_from_path(&mut self, path: &std::path::Path) -> bool {
        if !path.exists() {
            return false;
        }
        match image::ImageReader::open(path)
            .and_then(|r| r.with_guessed_format())
            .map_err(anyhow::Error::from)
            .and_then(|r| r.decode().map_err(anyhow::Error::from))
        {
            Ok(img) => {
                self.image_protocol = Some(self.picker.new_resize_protocol(img));
                true
            }
            Err(e) => {
                log::log(&format!("Failed to decode image: {}", e));
                self.flash(format!("Cannot preview: {}", e));
                false
            }
        }
    }

    pub fn spawn_cache_and_open(&self, key: &str) {
        let tx = self.bg_tx.clone();
        let bucket = self.bucket.clone();
        let key = key.to_string();
        let region = self.config.resolve_region(&bucket);
        let profile = self.config.resolve_profile(&bucket);
        std::thread::spawn(move || {
            // Check cache first
            if cache::is_cached(&bucket, &key) {
                let _ = tx.send(BgResult::CacheComplete { key, err: None });
                return;
            }
            let result = cache::download_to_cache(&bucket, &key, &region, &profile);
            let _ = tx.send(match result {
                Ok(_) => BgResult::CacheComplete { key, err: None },
                Err(e) => BgResult::CacheComplete {
                    key,
                    err: Some(e.to_string()),
                },
            });
        });
    }

    pub fn spawn_cache_for_preview(&self, key: &str) {
        let tx = self.bg_tx.clone();
        let bucket = self.bucket.clone();
        let key = key.to_string();
        let region = self.config.resolve_region(&bucket);
        let profile = self.config.resolve_profile(&bucket);
        std::thread::spawn(move || {
            if cache::is_cached(&bucket, &key) {
                let _ = tx.send(BgResult::CacheForPreview { key, err: None });
                return;
            }
            let result = cache::download_to_cache(&bucket, &key, &region, &profile);
            let _ = tx.send(match result {
                Ok(_) => BgResult::CacheForPreview { key, err: None },
                Err(e) => BgResult::CacheForPreview {
                    key,
                    err: Some(e.to_string()),
                },
            });
        });
    }

    pub fn spawn_load_restores(&self) {
        let tx = self.bg_tx.clone();
        std::thread::spawn(move || {
            let restores = restore::list_requests();
            let _ = tx.send(BgResult::RestoresLoaded { restores });
        });
    }

    pub fn spawn_check_all_restores(&self) {
        let tx = self.bg_tx.clone();
        std::thread::spawn(move || {
            let restores = restore::list_requests();
            for req in &restores {
                if req.status != "pending" {
                    continue;
                }
                let result = restore::check_restore(req);
                match result {
                    Ok(new_status) => {
                        let _ = restore::update_status(&req.id, &new_status, None);
                    }
                    Err(e) => {
                        // Still update last_checked_at even on error
                        let _ = restore::update_status(&req.id, &req.status, Some(&e.to_string()));
                    }
                }
            }
            // Reload after all checks
            let restores = restore::list_requests();
            let _ = tx.send(BgResult::RestoresLoaded { restores });
        });
    }

    // --- Library ---

    pub fn refresh_library(&mut self) {
        self.cached_files = cache::list_cached_files();
        self.clamp_library_selection();
    }

    pub fn filtered_library_items(&self) -> Vec<&cache::CachedFile> {
        if self.library_filter.is_empty() {
            self.cached_files.iter().collect()
        } else {
            let lower = self.library_filter.to_lowercase();
            self.cached_files
                .iter()
                .filter(|f| f.name.to_lowercase().contains(&lower))
                .collect()
        }
    }

    pub fn selected_cached_file(&self) -> Option<&cache::CachedFile> {
        let items = self.filtered_library_items();
        items.get(self.library_selected).copied()
    }

    fn clamp_library_selection(&mut self) {
        let len = self.filtered_library_items().len();
        if len == 0 {
            self.library_selected = 0;
        } else if self.library_selected >= len {
            self.library_selected = len - 1;
        }
    }
}

pub fn display_name<'a>(key: &'a str, prefix: &str) -> &'a str {
    let name = key.strip_prefix(prefix).unwrap_or(key);
    if name.is_empty() { key } else { name }
}

// Re-export shared helpers so existing TUI code and tests keep working.
pub use crate::helpers::{format_size, is_glacier};

pub fn storage_class_label(class: &str) -> &str {
    match class {
        "DEEP_ARCHIVE" => "DEEP",
        "GLACIER" => "GLACIER",
        "GLACIER_IR" => "GLCR_IR",
        "STANDARD" => "STD",
        "STANDARD_IA" => "STD_IA",
        "ONEZONE_IA" => "OZ_IA",
        "INTELLIGENT_TIERING" => "INT_T",
        other => other,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::helpers::{is_image, parent_prefix};

    // ── display_name ──

    #[test]
    fn display_name_strips_prefix() {
        assert_eq!(
            display_name("photos/2024/img.jpg", "photos/2024/"),
            "img.jpg"
        );
    }

    #[test]
    fn display_name_no_prefix() {
        assert_eq!(display_name("file.txt", ""), "file.txt");
    }

    #[test]
    fn display_name_prefix_equals_key() {
        // When strip results in empty, returns original key
        assert_eq!(display_name("photos/", "photos/"), "photos/");
    }

    #[test]
    fn display_name_no_match() {
        assert_eq!(display_name("other/file.txt", "photos/"), "other/file.txt");
    }

    // ── parent_prefix ──

    #[test]
    fn parent_prefix_nested() {
        assert_eq!(parent_prefix("a/b/c/"), "a/b/");
    }

    #[test]
    fn parent_prefix_one_level() {
        assert_eq!(parent_prefix("photos/"), "");
    }

    #[test]
    fn parent_prefix_empty() {
        assert_eq!(parent_prefix(""), "");
    }

    #[test]
    fn parent_prefix_no_trailing_slash() {
        assert_eq!(parent_prefix("a/b"), "a/");
    }

    #[test]
    fn parent_prefix_deep() {
        assert_eq!(parent_prefix("a/b/c/d/e/"), "a/b/c/d/");
    }

    // ── is_glacier ──

    #[test]
    fn is_glacier_true_cases() {
        assert!(is_glacier("GLACIER"));
        assert!(is_glacier("DEEP_ARCHIVE"));
        assert!(is_glacier("GLACIER_IR"));
    }

    #[test]
    fn is_glacier_false_cases() {
        assert!(!is_glacier("STANDARD"));
        assert!(!is_glacier("STANDARD_IA"));
        assert!(!is_glacier("ONEZONE_IA"));
        assert!(!is_glacier("INTELLIGENT_TIERING"));
        assert!(!is_glacier(""));
    }

    // ── format_size ──

    #[test]
    fn format_size_bytes() {
        assert_eq!(format_size(0), "0 B");
        assert_eq!(format_size(512), "512 B");
        assert_eq!(format_size(1023), "1023 B");
    }

    #[test]
    fn format_size_kb() {
        assert_eq!(format_size(1024), "1.0 KB");
        assert_eq!(format_size(1536), "1.5 KB");
    }

    #[test]
    fn format_size_mb() {
        assert_eq!(format_size(1024 * 1024), "1.0 MB");
        assert_eq!(format_size(5 * 1024 * 1024), "5.0 MB");
    }

    #[test]
    fn format_size_gb() {
        assert_eq!(format_size(1024 * 1024 * 1024), "1.0 GB");
    }

    #[test]
    fn format_size_tb() {
        assert_eq!(format_size(1024_i64 * 1024 * 1024 * 1024), "1.0 TB");
    }

    // ── storage_class_label ──

    #[test]
    fn storage_class_label_known() {
        assert_eq!(storage_class_label("DEEP_ARCHIVE"), "DEEP");
        assert_eq!(storage_class_label("GLACIER"), "GLACIER");
        assert_eq!(storage_class_label("GLACIER_IR"), "GLCR_IR");
        assert_eq!(storage_class_label("STANDARD"), "STD");
        assert_eq!(storage_class_label("STANDARD_IA"), "STD_IA");
        assert_eq!(storage_class_label("ONEZONE_IA"), "OZ_IA");
        assert_eq!(storage_class_label("INTELLIGENT_TIERING"), "INT_T");
    }

    #[test]
    fn storage_class_label_unknown_passthrough() {
        assert_eq!(
            storage_class_label("REDUCED_REDUNDANCY"),
            "REDUCED_REDUNDANCY"
        );
        assert_eq!(storage_class_label("CUSTOM"), "CUSTOM");
    }

    // ── is_image ──

    #[test]
    fn is_image_true_cases() {
        assert!(is_image("photo.jpg"));
        assert!(is_image("photo.jpeg"));
        assert!(is_image("photo.png"));
        assert!(is_image("photo.gif"));
        assert!(is_image("photo.bmp"));
        assert!(is_image("photo.webp"));
        assert!(is_image("photo.tiff"));
        assert!(is_image("photo.tif"));
    }

    #[test]
    fn is_image_case_insensitive() {
        assert!(is_image("PHOTO.JPG"));
        assert!(is_image("Photo.PNG"));
        assert!(is_image("img.Jpeg"));
    }

    #[test]
    fn is_image_with_path() {
        assert!(is_image("photos/2024/vacation/IMG_001.jpg"));
        assert!(is_image("deep/nested/path/file.png"));
    }

    #[test]
    fn is_image_false_cases() {
        assert!(!is_image("file.txt"));
        assert!(!is_image("archive.tar.gz"));
        assert!(!is_image("document.pdf"));
        assert!(!is_image("script.rs"));
        assert!(!is_image("data.json"));
        assert!(!is_image("prefix/"));
        assert!(!is_image(""));
    }
}
