use ratatui::Frame;
use ratatui::layout::{Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, Borders, Clear, Paragraph, Row, Table};

use crate::app::{
    App, ConfirmAction, Mode, SPINNER, Tab, display_name, format_size, is_glacier,
    storage_class_label,
};
use ratatui_image::StatefulImage;

use crate::cache;

const CYAN: Color = Color::Cyan;
const GREEN: Color = Color::Green;
const RED: Color = Color::Red;
const YELLOW: Color = Color::Yellow;
const DIM: Color = Color::DarkGray;
const ORANGE: Color = Color::Rgb(255, 165, 0);
const BLUE: Color = Color::Blue;
const SELECT_BG: Color = Color::Rgb(30, 40, 55);

pub fn draw(f: &mut Frame, app: &mut App) {
    let chunks = Layout::vertical([
        Constraint::Length(2), // header + separator
        Constraint::Min(5),    // content
        Constraint::Length(2), // status bar
        Constraint::Length(1), // keybindings
    ])
    .split(f.area());

    draw_header(f, app, chunks[0]);

    if app.loading.is_some() {
        draw_loading(f, app, chunks[1]);
    } else {
        match app.tab {
            Tab::Browse => draw_browse(f, app, chunks[1]),
            Tab::Profiles => draw_profiles(f, app, chunks[1]),
            Tab::Restores => draw_restores(f, app, chunks[1]),
            Tab::Library => draw_library(f, app, chunks[1]),
        }
    }

    draw_status_bar(f, app, chunks[2]);
    draw_keybindings(f, app, chunks[3]);

    // Overlays
    match &app.mode {
        Mode::BucketPicker { .. } => draw_bucket_picker(f, app),
        Mode::Confirm { .. } => draw_confirm(f, app),
        Mode::Detail { .. } => draw_detail(f, app),
        Mode::LinkBucket { .. } => draw_link_form(f, app),
        Mode::AddProfile { .. } => draw_profile_form(f, app, "Add Profile"),
        Mode::EditProfile { .. } => draw_profile_form(f, app, "Edit Profile"),
        Mode::Help => draw_help(f),
        Mode::TierPicker { .. } => draw_tier_picker(f, app),
        Mode::ImagePreview { .. } => draw_image_preview(f, app),
        Mode::Rename { .. } => draw_rename(f, app),
        Mode::LibraryInfo { .. } => draw_library_info(f, app),
        Mode::Upload { .. } => draw_upload_form(f, app),
        Mode::ScpPicker { .. } => draw_scp_picker(f, app),
        Mode::Normal | Mode::Filter => {}
    }
}

// ---------------------------------------------------------------------------
// Header
// ---------------------------------------------------------------------------

fn draw_header(f: &mut Frame, app: &App, area: Rect) {
    let chunks = Layout::vertical([Constraint::Length(1), Constraint::Length(1)]).split(area);

    let title = Span::styled(
        " yelo ",
        Style::default().fg(CYAN).add_modifier(Modifier::BOLD),
    );

    let tab = |label: &str, num: &str, active: bool| -> Vec<Span> {
        let (num_style, label_style) = if active {
            (
                Style::default().fg(Color::Black).bg(CYAN),
                Style::default().fg(CYAN).add_modifier(Modifier::BOLD),
            )
        } else {
            (
                Style::default().fg(Color::Black).bg(DIM),
                Style::default().fg(DIM),
            )
        };
        vec![
            Span::styled(format!(" {} ", num), num_style),
            Span::styled(format!(" {} ", label), label_style),
        ]
    };

    let mut spans = vec![title, Span::raw("  ")];
    spans.extend(tab("Browse", "1", app.tab == Tab::Browse));
    spans.push(Span::raw(" "));
    spans.extend(tab("Profiles", "2", app.tab == Tab::Profiles));
    spans.push(Span::raw(" "));
    spans.extend(tab("Restores", "3", app.tab == Tab::Restores));
    spans.push(Span::raw(" "));
    spans.extend(tab("Library", "4", app.tab == Tab::Library));

    f.render_widget(Paragraph::new(Line::from(spans)), chunks[0]);
    f.render_widget(
        Paragraph::new("─".repeat(area.width as usize)).style(Style::default().fg(DIM)),
        chunks[1],
    );
}

// ---------------------------------------------------------------------------
// Browse tab
// ---------------------------------------------------------------------------

fn draw_browse(f: &mut Frame, app: &App, area: Rect) {
    if app.state.bucket.is_empty() {
        let msg = Paragraph::new("No bucket selected. Press b to pick a bucket.")
            .style(Style::default().fg(DIM));
        f.render_widget(msg, area);
        return;
    }

    let content_area = if matches!(app.mode, Mode::Filter) {
        let chunks = Layout::vertical([Constraint::Length(1), Constraint::Min(1)]).split(area);
        let filter_line = Line::from(vec![
            Span::styled("filter: ", Style::default().fg(CYAN)),
            Span::styled(&app.filter_text, Style::default().fg(Color::White)),
            Span::styled("_", Style::default().fg(CYAN)),
        ]);
        f.render_widget(Paragraph::new(filter_line), chunks[0]);
        chunks[1]
    } else {
        area
    };

    let filtered = app.filtered_items();
    if filtered.is_empty() {
        let msg = Paragraph::new("  (empty)").style(Style::default().fg(DIM));
        f.render_widget(msg, content_area);
        return;
    }

    let name_width = content_area.width.saturating_sub(20);
    let widths = [
        Constraint::Length(name_width.max(20)),
        Constraint::Length(8),
        Constraint::Length(10),
    ];
    let header = Row::new(vec!["Name", "Class", "Size"])
        .style(Style::default().fg(CYAN).add_modifier(Modifier::BOLD))
        .bottom_margin(0);

    let visible_height = content_area.height.saturating_sub(1) as usize;
    // Adjust scroll to keep selection visible
    let scroll = {
        let mut s = app.browse_scroll;
        if app.browse_selected < s {
            s = app.browse_selected;
        }
        if app.browse_selected >= s + visible_height {
            s = app.browse_selected.saturating_sub(visible_height - 1);
        }
        s
    };

    let rows: Vec<Row> = filtered
        .iter()
        .enumerate()
        .skip(scroll)
        .take(visible_height)
        .map(|(i, item)| {
            let name = display_name(&item.key, &app.state.prefix);
            let (name_line, class_span, size_str) = if item.is_prefix {
                (
                    Line::from(Span::styled(name, Style::default().fg(CYAN))),
                    Span::styled("PRE", Style::default().fg(DIM)),
                    String::new(),
                )
            } else {
                let class_color = storage_class_color(&item.storage_class);
                let cached = cache::is_cached(&app.state.bucket, &item.key);
                let name_line = if cached {
                    Line::from(vec![
                        Span::styled("● ", Style::default().fg(GREEN)),
                        Span::raw(name),
                    ])
                } else {
                    Line::from(Span::raw(name))
                };
                (
                    name_line,
                    Span::styled(
                        storage_class_label(&item.storage_class),
                        Style::default().fg(class_color),
                    ),
                    format_size(item.size),
                )
            };

            let row = Row::new(vec![
                ratatui::text::Text::from(name_line),
                ratatui::text::Text::from(class_span),
                ratatui::text::Text::from(size_str),
            ]);

            if i == app.browse_selected {
                row.style(Style::default().bg(SELECT_BG).add_modifier(Modifier::BOLD))
            } else {
                row
            }
        })
        .collect();

    let table = Table::new(rows, widths).header(header);
    f.render_widget(table, content_area);
}

// ---------------------------------------------------------------------------
// Profiles tab
// ---------------------------------------------------------------------------

fn draw_profiles(f: &mut Frame, app: &App, area: Rect) {
    if app.profiles.is_empty() {
        let lines = vec![
            Line::from(""),
            Line::from("  No AWS profiles found."),
            Line::from(""),
            Line::from(vec![
                Span::raw("  Press "),
                Span::styled("a", Style::default().fg(CYAN)),
                Span::raw(" to add a new profile."),
            ]),
            Line::from("  Profiles are stored in ~/.aws/credentials and ~/.aws/config."),
        ];
        f.render_widget(Paragraph::new(lines).style(Style::default().fg(DIM)), area);
        return;
    }

    let widths = [
        Constraint::Length(20),
        Constraint::Length(24),
        Constraint::Length(16),
        Constraint::Min(10),
    ];
    let header = Row::new(vec!["Profile", "Linked Bucket", "Region", "Status"])
        .style(Style::default().fg(CYAN).add_modifier(Modifier::BOLD));

    let rows: Vec<Row> = app
        .profiles
        .iter()
        .enumerate()
        .map(|(i, profile)| {
            let mut bucket_display = String::new();
            let mut region_display = String::new();

            for b in &app.config.buckets {
                let p = if b.profile.is_empty() {
                    "default"
                } else {
                    &b.profile
                };
                if p == profile {
                    bucket_display = b.name.clone();
                    if b.name == app.config.default_bucket {
                        bucket_display.push_str(" *");
                    }
                    region_display = b.region.clone();
                    break;
                }
            }

            let bucket_span = if bucket_display.is_empty() {
                Span::styled("(none)", Style::default().fg(DIM))
            } else {
                Span::raw(bucket_display)
            };
            let region_span = if region_display.is_empty() {
                Span::styled("(default)", Style::default().fg(DIM))
            } else {
                Span::raw(region_display)
            };

            let status_span = match app.profile_status.get(profile).map(|s| s.as_str()) {
                Some("ok") => Span::styled("● connected", Style::default().fg(GREEN)),
                Some("fail") => Span::styled("● failed", Style::default().fg(RED)),
                Some("testing") => Span::styled("● testing...", Style::default().fg(YELLOW)),
                _ => Span::raw(""),
            };

            let row = Row::new(vec![
                ratatui::text::Text::from(profile.as_str()),
                ratatui::text::Text::from(bucket_span),
                ratatui::text::Text::from(region_span),
                ratatui::text::Text::from(status_span),
            ]);

            if i == app.profile_selected {
                row.style(Style::default().bg(SELECT_BG).add_modifier(Modifier::BOLD))
            } else {
                row
            }
        })
        .collect();

    let table = Table::new(rows, widths).header(header);
    f.render_widget(table, area);
}

// ---------------------------------------------------------------------------
// Restores tab
// ---------------------------------------------------------------------------

fn draw_restores(f: &mut Frame, app: &App, area: Rect) {
    if app.restores.is_empty() {
        let lines = vec![
            Line::from(""),
            Line::from("  No restore requests tracked."),
            Line::from(""),
            Line::from(vec![
                Span::raw("  Browse to a Glacier object and press "),
                Span::styled("r", Style::default().fg(CYAN)),
                Span::raw(" to initiate a restore."),
            ]),
        ];
        f.render_widget(Paragraph::new(lines).style(Style::default().fg(DIM)), area);
        return;
    }

    let widths = [
        Constraint::Min(20),
        Constraint::Length(16),
        Constraint::Length(12),
        Constraint::Length(16),
        Constraint::Length(16),
        Constraint::Length(10),
    ];
    let header = Row::new(vec![
        "Key",
        "Bucket",
        "Status",
        "Requested",
        "Last Checked",
        "Tier",
    ])
    .style(Style::default().fg(CYAN).add_modifier(Modifier::BOLD));

    let visible_height = area.height.saturating_sub(1) as usize;
    let scroll = {
        let mut s = app.restore_scroll;
        if app.restore_selected < s {
            s = app.restore_selected;
        }
        if app.restore_selected >= s + visible_height {
            s = app.restore_selected.saturating_sub(visible_height - 1);
        }
        s
    };

    let rows: Vec<Row> = app
        .restores
        .iter()
        .enumerate()
        .skip(scroll)
        .take(visible_height)
        .map(|(i, req)| {
            let name = req.key.rsplit('/').next().unwrap_or(&req.key);
            let (status_label, status_color) = restore_status_style(&req.status);
            let requested = req.requested_at.format("%m-%d %H:%M").to_string();
            let last_checked = req
                .last_checked_at
                .map(|t| {
                    t.with_timezone(&chrono::Local)
                        .format("%m-%d %H:%M")
                        .to_string()
                })
                .unwrap_or_else(|| "—".to_string());

            let row = Row::new(vec![
                ratatui::text::Text::from(name.to_string()),
                ratatui::text::Text::from(req.bucket.clone()),
                ratatui::text::Text::from(Span::styled(
                    status_label,
                    Style::default().fg(status_color),
                )),
                ratatui::text::Text::from(requested),
                ratatui::text::Text::from(last_checked),
                ratatui::text::Text::from(req.tier.clone()),
            ]);

            if i == app.restore_selected {
                row.style(Style::default().bg(SELECT_BG).add_modifier(Modifier::BOLD))
            } else {
                row
            }
        })
        .collect();

    let table = Table::new(rows, widths).header(header);
    f.render_widget(table, area);
}

fn restore_status_style(status: &crate::restore::RestoreStatus) -> (&'static str, Color) {
    use crate::restore::RestoreStatus;
    match status {
        RestoreStatus::Pending => ("pending", YELLOW),
        RestoreStatus::Available => ("available", GREEN),
        RestoreStatus::Downloaded => ("downloaded", CYAN),
        RestoreStatus::Failed => ("failed", RED),
        RestoreStatus::Cancelled => ("cancelled", DIM),
    }
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

fn draw_loading(f: &mut Frame, app: &App, area: Rect) {
    let frame = SPINNER[app.spinner_tick % SPINNER.len()];
    let msg = app.loading.as_deref().unwrap_or("Loading...");
    let line = Line::from(vec![
        Span::styled(format!("{} ", frame), Style::default().fg(CYAN)),
        Span::styled(msg, Style::default().fg(CYAN)),
    ]);
    f.render_widget(Paragraph::new(line), area);
}

// ---------------------------------------------------------------------------
// Status bar
// ---------------------------------------------------------------------------

fn draw_status_bar(f: &mut Frame, app: &App, area: Rect) {
    let chunks = Layout::vertical([Constraint::Length(1), Constraint::Length(1)]).split(area);

    f.render_widget(
        Paragraph::new("─".repeat(area.width as usize)).style(Style::default().fg(DIM)),
        chunks[0],
    );

    let line = if let Some(ref loading_msg) = app.loading {
        let frame = SPINNER[app.spinner_tick % SPINNER.len()];
        Line::from(vec![
            Span::styled(format!(" {} ", frame), Style::default().fg(CYAN)),
            Span::styled(loading_msg.as_str(), Style::default().fg(CYAN)),
        ])
    } else if let Some(ref msg) = app.status_msg {
        let color = if msg.starts_with("Error") || msg.contains("failed") {
            RED
        } else {
            YELLOW
        };
        Line::from(Span::styled(
            format!(" {}", msg),
            Style::default().fg(color),
        ))
    } else if app.tab == Tab::Browse && !app.state.bucket.is_empty() {
        Line::from(Span::styled(
            format!(" {}:/{}", app.state.bucket, app.state.prefix),
            Style::default().fg(DIM),
        ))
    } else if app.tab == Tab::Profiles {
        Line::from(Span::styled(
            " Profiles from ~/.aws/ — credentials managed inline",
            Style::default().fg(DIM),
        ))
    } else if app.tab == Tab::Restores {
        let pending = app
            .restores
            .iter()
            .filter(|r| r.status == crate::restore::RestoreStatus::Pending)
            .count();
        let daemon_indicator = if crate::daemon::is_running() {
            " — daemon active"
        } else {
            ""
        };
        Line::from(Span::styled(
            format!(
                " {} restores tracked — {} pending{}",
                app.restores.len(),
                pending,
                daemon_indicator
            ),
            Style::default().fg(DIM),
        ))
    } else if app.tab == Tab::Library {
        let total_bytes: u64 = app.cached_files.iter().map(|f| f.size).sum();
        let total_str = format_size(total_bytes as i64);
        Line::from(Span::styled(
            format!(" {} files — {}", app.cached_files.len(), total_str),
            Style::default().fg(DIM),
        ))
    } else {
        Line::from("")
    };

    f.render_widget(Paragraph::new(line), chunks[1]);
}

// ---------------------------------------------------------------------------
// Keybinding bar
// ---------------------------------------------------------------------------

fn draw_keybindings(f: &mut Frame, app: &App, area: Rect) {
    let keys: Vec<(&str, &str)> = match &app.mode {
        Mode::Help => vec![("esc", "close")],
        Mode::BucketPicker { .. } => vec![("j/k", "nav"), ("enter", "select"), ("esc", "cancel")],
        Mode::Confirm { .. } => vec![("y", "confirm"), ("esc", "cancel")],
        Mode::Detail { .. } => vec![
            ("esc", "close"),
            ("o", "view"),
            ("g", "download"),
            ("r", "restore"),
        ],
        Mode::Filter => vec![("enter/esc", "done")],
        Mode::TierPicker { .. } => vec![("j/k", "nav"), ("enter", "select"), ("esc", "cancel")],
        Mode::ImagePreview { .. } => {
            vec![("o", "open native"), ("s", "metadata"), ("esc", "close")]
        }
        Mode::LinkBucket { .. } | Mode::AddProfile { .. } | Mode::EditProfile { .. } => {
            vec![("tab", "next"), ("enter", "save"), ("esc", "cancel")]
        }
        Mode::Upload { .. } => {
            vec![("tab", "next"), ("enter", "upload"), ("esc", "cancel")]
        }
        Mode::ScpPicker { .. } => {
            vec![("↑↓", "host"), ("enter", "send"), ("esc", "cancel")]
        }
        Mode::Rename { .. } => vec![("enter", "save"), ("esc", "cancel")],
        Mode::LibraryInfo { .. } => vec![("esc", "close")],
        Mode::Normal => match (app.tab, app.submenu) {
            (Tab::Browse, false) => vec![
                ("j/k", "nav"),
                ("enter", "open"),
                ("o", "view"),
                ("h", "back"),
                ("/", "filter"),
                (".", "more"),
                ("q", "quit"),
            ],
            (Tab::Browse, true) => vec![
                ("g", "get"),
                ("u", "upload"),
                ("r", "restore"),
                ("s", "stat"),
                ("b", "buckets"),
                ("R", "refresh"),
                ("?", "help"),
                (".", "back"),
            ],
            (Tab::Profiles, false) => vec![
                ("j/k", "nav"),
                ("a", "add"),
                ("e", "edit"),
                ("t", "test"),
                (".", "more"),
                ("q", "quit"),
            ],
            (Tab::Profiles, true) => vec![
                ("l", "link"),
                ("d", "delete"),
                ("D", "default"),
                ("u", "unlink"),
                ("R", "refresh"),
                ("?", "help"),
                (".", "back"),
            ],
            (Tab::Restores, false) => vec![
                ("j/k", "nav"),
                ("c", "check"),
                ("enter", "download"),
                (".", "more"),
                ("q", "quit"),
            ],
            (Tab::Restores, true) => {
                let daemon_label = if crate::daemon::is_running() {
                    "stop daemon"
                } else {
                    "start daemon"
                };
                vec![
                    ("x", "cancel"),
                    ("D", daemon_label),
                    ("R", "refresh"),
                    ("d", "delete"),
                    ("?", "help"),
                    (".", "back"),
                ]
            }
            (Tab::Library, false) => vec![
                ("j/k", "nav"),
                ("o", "open"),
                ("d", "delete"),
                ("r", "rename"),
                ("s", "copy path"),
                ("i", "info"),
                ("/", "filter"),
                (".", "more"),
                ("q", "quit"),
            ],
            (Tab::Library, true) => vec![
                ("l", "presign URL"),
                ("c", "scp to"),
                ("R", "refresh"),
                ("?", "help"),
                (".", "back"),
            ],
        },
    };

    let spans: Vec<Span> = keys
        .iter()
        .flat_map(|(key, desc)| {
            vec![
                Span::styled(
                    format!(" {} ", key),
                    Style::default().fg(Color::Black).bg(DIM),
                ),
                Span::styled(format!(" {} ", desc), Style::default().fg(DIM)),
            ]
        })
        .collect();

    f.render_widget(Paragraph::new(Line::from(spans)), area);
}

// ---------------------------------------------------------------------------
// Overlays
// ---------------------------------------------------------------------------

fn fixed_centered_rect(percent_x: u16, height: u16, area: Rect) -> Rect {
    let y = area.y + (area.height.saturating_sub(height)) / 2;
    let x_margin = (area.width as u32 * (100 - percent_x as u32) / 100 / 2) as u16;
    let w = area.width.saturating_sub(x_margin * 2);
    Rect::new(area.x + x_margin, y, w, height.min(area.height))
}

fn draw_overlay_block(f: &mut Frame, title: &str, area: Rect) -> Rect {
    f.render_widget(Clear, area);
    let block = Block::default()
        .title(format!(" {} ", title))
        .title_style(Style::default().fg(CYAN).add_modifier(Modifier::BOLD))
        .borders(Borders::ALL)
        .border_style(Style::default().fg(CYAN));
    let inner = block.inner(area);
    f.render_widget(block, area);
    inner
}

fn draw_bucket_picker(f: &mut Frame, app: &App) {
    let Mode::BucketPicker { buckets, selected } = &app.mode else {
        return;
    };
    let h = (buckets.len() as u16 + 4).min(20);
    let area = fixed_centered_rect(50, h, f.area());
    let inner = draw_overlay_block(f, "Select Bucket", area);

    let rows: Vec<Row> = buckets
        .iter()
        .enumerate()
        .map(|(i, b)| {
            let label = if *b == app.state.bucket {
                format!("* {}", b)
            } else {
                format!("  {}", b)
            };
            let row = Row::new(vec![label]);
            if i == *selected {
                row.style(Style::default().bg(SELECT_BG).add_modifier(Modifier::BOLD))
            } else {
                row
            }
        })
        .collect();

    let table = Table::new(rows, [Constraint::Percentage(100)]);
    f.render_widget(table, inner);
}

fn draw_confirm(f: &mut Frame, app: &App) {
    let Mode::Confirm { action, target } = &app.mode else {
        return;
    };
    let action_str = match action {
        ConfirmAction::Download => "download",
        ConfirmAction::DeleteProfile => "delete profile",
        ConfirmAction::CancelRestore => "cancel restore",
        ConfirmAction::DeleteRestore => "delete restore record",
        ConfirmAction::DeleteCached => "delete cached file",
    };
    let name = if *action == ConfirmAction::DeleteCached {
        // target is "bucket\0key" — extract filename from key part
        target
            .split_once('\0')
            .map(|(_, key)| key.rsplit('/').next().unwrap_or(key))
            .unwrap_or(target)
    } else {
        target.rsplit('/').next().unwrap_or(target)
    };

    let area = fixed_centered_rect(50, 7, f.area());
    let inner = draw_overlay_block(f, "Confirm", area);

    let lines = vec![
        Line::from(""),
        Line::from(format!("  {} '{}' ?", action_str, name)),
        Line::from(""),
        Line::from(Span::styled(
            "  Press y to confirm, esc to cancel",
            Style::default().fg(DIM),
        )),
    ];
    f.render_widget(Paragraph::new(lines), inner);
}

fn draw_detail(f: &mut Frame, app: &App) {
    let Mode::Detail { info } = &app.mode else {
        return;
    };
    let area = fixed_centered_rect(60, 16, f.area());
    let inner = draw_overlay_block(f, "Object Detail", area);

    let field = |label: &str, value: &str| -> Line<'static> {
        Line::from(vec![
            Span::styled(format!("{:>12}  ", label), Style::default().fg(DIM)),
            Span::raw(value.to_string()),
        ])
    };

    let class_color = storage_class_color(&info.storage_class);
    let name = info.key.rsplit('/').next().unwrap_or(&info.key);

    let mut lines = vec![
        Line::from(""),
        field("Key", name),
        field("Path", &info.key),
        field(
            "Size",
            &format!("{} ({} B)", format_size(info.size), info.size),
        ),
        Line::from(vec![
            Span::styled(format!("{:>12}  ", "Class"), Style::default().fg(DIM)),
            Span::styled(
                storage_class_label(&info.storage_class),
                Style::default().fg(class_color),
            ),
        ]),
        field("Modified", &info.last_modified),
    ];

    if !info.content_type.is_empty() {
        lines.push(field("Type", &info.content_type));
    }
    if !info.etag.is_empty() {
        lines.push(field("ETag", &info.etag));
    }
    if cache::is_cached(&app.state.bucket, &info.key) {
        lines.push(Line::from(vec![
            Span::styled(format!("{:>12}  ", "Cache"), Style::default().fg(DIM)),
            Span::styled("● cached", Style::default().fg(GREEN)),
        ]));
    }
    if is_glacier(&info.storage_class) {
        let (label, color) = match info.restore_status.as_str() {
            "in-progress" => ("restoring", YELLOW),
            "available" => ("available", GREEN),
            _ => ("not restored", DIM),
        };
        lines.push(Line::from(vec![
            Span::styled(format!("{:>12}  ", "Restore"), Style::default().fg(DIM)),
            Span::styled(label, Style::default().fg(color)),
        ]));
    }

    f.render_widget(Paragraph::new(lines), inner);
}

fn draw_link_form(f: &mut Frame, app: &App) {
    let Mode::LinkBucket {
        focus,
        bucket,
        region,
        profile,
    } = &app.mode
    else {
        return;
    };
    let area = fixed_centered_rect(55, 9, f.area());
    let inner = draw_overlay_block(f, "Link Bucket to Profile", area);

    let fields = [
        ("Bucket:", bucket.as_str(), 0),
        ("Region:", region.as_str(), 1),
        ("Profile:", profile.as_str(), 2),
    ];

    let chunks = Layout::vertical([
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
    ])
    .split(inner);

    for (i, (label, value, _)) in fields.iter().enumerate() {
        let cursor = if *focus == i { "_" } else { "" };
        let style = if *focus == i {
            Style::default().fg(CYAN)
        } else {
            Style::default().fg(DIM)
        };
        let line = Line::from(vec![
            Span::styled(format!("  {:10}", label), style),
            Span::styled(
                format!("{}{}", value, cursor),
                Style::default().fg(Color::White),
            ),
        ]);
        f.render_widget(Paragraph::new(line), chunks[i + 1]);
    }
}

fn draw_upload_form(f: &mut Frame, app: &App) {
    let Mode::Upload {
        focus,
        local_path,
        key,
        storage_class,
        completions,
        comp_selected,
    } = &app.mode
    else {
        return;
    };

    let max_comp = 8;
    let comp_height = completions.len().min(max_comp) as u16;
    let form_height = 7 + if comp_height > 0 { comp_height + 1 } else { 0 };

    let title = format!("Upload to s3://{}/", app.state.bucket);
    let area = fixed_centered_rect(65, form_height, f.area());
    let inner = draw_overlay_block(f, &title, area);

    let mut constraints = vec![
        Constraint::Length(1), // padding
        Constraint::Length(1), // File field
        Constraint::Length(1), // Key field
        Constraint::Length(1), // Class field
        Constraint::Length(1), // padding
    ];
    if comp_height > 0 {
        constraints.push(Constraint::Length(comp_height));
    }

    let chunks = Layout::vertical(constraints).split(inner);

    let fields = [
        ("File:", local_path.as_str(), 0),
        ("Key:", key.as_str(), 1),
        ("Class:", storage_class.as_str(), 2),
    ];

    for (i, (label, value, _)) in fields.iter().enumerate() {
        let cursor = if *focus == i { "_" } else { "" };
        let style = if *focus == i {
            Style::default().fg(CYAN)
        } else {
            Style::default().fg(DIM)
        };
        let line = Line::from(vec![
            Span::styled(format!("  {:10}", label), style),
            Span::styled(
                format!("{}{}", value, cursor),
                Style::default().fg(Color::White),
            ),
        ]);
        f.render_widget(Paragraph::new(line), chunks[i + 1]);
    }

    // Draw completion list below the form fields
    if comp_height > 0 {
        let comp_area = chunks[5];
        let visible: Vec<Line> = completions
            .iter()
            .take(max_comp)
            .enumerate()
            .map(|(i, entry)| {
                let selected = *comp_selected == Some(i);
                let is_dir = entry.ends_with('/');
                let style = if selected {
                    Style::default().bg(SELECT_BG).fg(Color::White)
                } else if is_dir {
                    Style::default().fg(CYAN)
                } else {
                    Style::default().fg(DIM)
                };
                Line::styled(format!("  {}", entry), style)
            })
            .collect();
        f.render_widget(Paragraph::new(visible), comp_area);
    }
}

fn draw_scp_picker(f: &mut Frame, app: &App) {
    let Mode::ScpPicker {
        hosts,
        selected,
        dest,
        cached_path: _,
    } = &app.mode
    else {
        return;
    };

    let visible = hosts.len().min(10);
    let height = (visible as u16) + 6; // title + hosts + blank + dest label + dest field + padding
    let area = fixed_centered_rect(55, height, f.area());
    let inner = draw_overlay_block(f, "SCP — send to:", area);

    let mut constraints: Vec<Constraint> = Vec::new();
    for _ in 0..visible {
        constraints.push(Constraint::Length(1));
    }
    constraints.push(Constraint::Length(1)); // blank
    constraints.push(Constraint::Length(1)); // dest label
    constraints.push(Constraint::Length(1)); // dest input

    let chunks = Layout::vertical(constraints).split(inner);

    for (i, host) in hosts.iter().take(visible).enumerate() {
        let style = if i == *selected {
            Style::default().bg(SELECT_BG).fg(Color::White)
        } else {
            Style::default().fg(DIM)
        };
        let marker = if i == *selected { "▸ " } else { "  " };
        let line = Line::styled(format!("{}{}", marker, host), style);
        f.render_widget(Paragraph::new(line), chunks[i]);
    }

    let dest_label = Line::from(Span::styled("  Dest:", Style::default().fg(CYAN)));
    f.render_widget(Paragraph::new(dest_label), chunks[visible + 1]);

    let dest_line = Line::from(Span::styled(
        format!("  {}_", dest),
        Style::default().fg(Color::White),
    ));
    f.render_widget(Paragraph::new(dest_line), chunks[visible + 2]);
}

fn draw_profile_form(f: &mut Frame, app: &App, title: &str) {
    let (focus, name, access_key, secret_key, region) = match &app.mode {
        Mode::AddProfile {
            focus,
            name,
            access_key,
            secret_key,
            region,
        } => (
            *focus,
            name.as_str(),
            access_key.as_str(),
            secret_key.as_str(),
            region.as_str(),
        ),
        Mode::EditProfile {
            focus,
            name,
            access_key,
            secret_key,
            region,
        } => (
            *focus,
            name.as_str(),
            access_key.as_str(),
            secret_key.as_str(),
            region.as_str(),
        ),
        _ => return,
    };

    let is_edit = matches!(app.mode, Mode::EditProfile { .. });
    let h = if is_edit { 12 } else { 10 };
    let area = fixed_centered_rect(60, h, f.area());
    let inner = draw_overlay_block(f, title, area);

    let masked_secret: String = "•".repeat(secret_key.len());

    let fields: Vec<(&str, &str, usize)> = vec![
        ("Profile:", name, 0),
        ("Access Key:", access_key, 1),
        ("Secret Key:", &masked_secret, 2),
        ("Region:", region, 3),
    ];

    let mut constraints: Vec<Constraint> = vec![Constraint::Length(1)]; // top padding
    for _ in &fields {
        constraints.push(Constraint::Length(1));
    }
    if is_edit {
        constraints.push(Constraint::Length(1)); // hint
    }
    constraints.push(Constraint::Length(1)); // bottom padding

    let chunks = Layout::vertical(constraints).split(inner);

    for (i, (label, value, _)) in fields.iter().enumerate() {
        let cursor = if focus == i { "_" } else { "" };
        let style = if focus == i {
            Style::default().fg(CYAN).add_modifier(Modifier::BOLD)
        } else {
            Style::default().fg(DIM)
        };
        let val_style = if focus == i {
            Style::default().fg(Color::White)
        } else {
            Style::default().fg(DIM)
        };
        let line = Line::from(vec![
            Span::styled(format!("  {:14}", label), style),
            Span::styled(format!("{}{}", value, cursor), val_style),
        ]);
        f.render_widget(Paragraph::new(line), chunks[i + 1]);
    }

    if is_edit {
        let hint_idx = fields.len() + 1;
        f.render_widget(
            Paragraph::new(Span::styled(
                "  Leave Secret Key empty to keep existing",
                Style::default().fg(DIM),
            )),
            chunks[hint_idx],
        );
    }
}

fn draw_help(f: &mut Frame) {
    let area = fixed_centered_rect(65, 46, f.area());
    let inner = draw_overlay_block(f, "Help", area);

    let help_items: Vec<(&str, &str)> = vec![
        ("", "── Browse ──"),
        ("1/2/3/4", "Switch tabs"),
        ("j/k ↑/↓", "Navigate"),
        ("enter", "Open prefix or view detail"),
        ("o", "Cache & open in default app"),
        ("h / bksp", "Go to parent directory"),
        ("g", "Download selected object"),
        ("r", "Initiate Glacier restore"),
        ("b", "Switch bucket"),
        ("/", "Filter listing"),
        ("", ""),
        ("", "── Browse . submenu ──"),
        ("R", "Refresh listing"),
        ("u", "Upload local file"),
        ("s", "Object stat/metadata"),
        ("", ""),
        ("", "── Profiles ──"),
        ("a", "Add new AWS profile"),
        ("e", "Edit selected profile"),
        ("t", "Test profile connectivity"),
        ("l", "Link a bucket to a profile"),
        ("", ""),
        ("", "── Profiles . submenu ──"),
        ("d", "Delete profile"),
        ("D", "Set default bucket"),
        ("u", "Unlink bucket from profile"),
        ("S", "Configure SSO (shells out)"),
        ("R", "Refresh profiles"),
        ("", ""),
        ("", "── Restores ──"),
        ("c", "Check restore status"),
        ("enter", "Download if available"),
        ("", ""),
        ("", "── Restores . submenu ──"),
        ("x", "Cancel pending restore"),
        ("D", "Start/stop auto-download daemon"),
        ("d", "Delete completed record"),
        ("R", "Refresh all restores"),
        ("", ""),
        ("", "── Library ──"),
        ("o", "Open file in default app"),
        ("d", "Delete cached file"),
        ("r", "Rename file"),
        ("s", "Copy path to clipboard"),
        ("i", "File info"),
        ("p", "Image preview"),
        ("/", "Filter"),
        ("", ""),
        ("", "── Library . submenu ──"),
        ("l", "Copy presigned URL to clipboard"),
        ("c", "SCP file to another host"),
        ("R", "Refresh library"),
        ("", ""),
        (".", "Toggle submenu"),
        ("?", "This help"),
        ("q", "Quit (saves state)"),
    ];

    let lines: Vec<Line> = help_items
        .iter()
        .map(|(key, desc)| {
            if key.is_empty() && desc.starts_with("──") {
                // Section header
                Line::from(Span::styled(
                    format!("  {}", desc),
                    Style::default().fg(DIM),
                ))
            } else if key.is_empty() {
                Line::from("")
            } else {
                Line::from(vec![
                    Span::styled(format!("  {:14}", key), Style::default().fg(CYAN)),
                    Span::raw(*desc),
                ])
            }
        })
        .collect();

    f.render_widget(Paragraph::new(lines), inner);
}

fn draw_image_preview(f: &mut Frame, app: &mut App) {
    let Mode::ImagePreview { key } = &app.mode else {
        return;
    };
    let name = key.rsplit('/').next().unwrap_or(key).to_string();
    let cached = cache::is_cached(&app.state.bucket, key);

    let area = f.area();
    f.render_widget(Clear, area);

    let chunks = Layout::vertical([
        Constraint::Length(2), // title bar
        Constraint::Min(5),    // image
        Constraint::Length(1), // info line
    ])
    .split(area);

    // Title
    let title_spans = vec![
        Span::styled(" ", Style::default()),
        Span::styled(
            &name,
            Style::default().fg(CYAN).add_modifier(Modifier::BOLD),
        ),
        if cached {
            Span::styled("  ● cached", Style::default().fg(GREEN))
        } else {
            Span::raw("")
        },
    ];
    f.render_widget(Paragraph::new(Line::from(title_spans)), chunks[0]);

    // Image
    if let Some(ref mut protocol) = app.image_protocol {
        let image_widget = StatefulImage::default();
        f.render_stateful_widget(image_widget, chunks[1], protocol);
    } else {
        f.render_widget(
            Paragraph::new("  (no preview available)").style(Style::default().fg(DIM)),
            chunks[1],
        );
    }

    // Info line
    let info = Line::from(vec![
        Span::styled(" o", Style::default().fg(Color::Black).bg(DIM)),
        Span::styled(" open ", Style::default().fg(DIM)),
        Span::styled(" s", Style::default().fg(Color::Black).bg(DIM)),
        Span::styled(" metadata ", Style::default().fg(DIM)),
        Span::styled(" esc", Style::default().fg(Color::Black).bg(DIM)),
        Span::styled(" close ", Style::default().fg(DIM)),
    ]);
    f.render_widget(Paragraph::new(info), chunks[2]);
}

fn draw_library(f: &mut Frame, app: &App, area: Rect) {
    let content_area = if matches!(app.mode, Mode::Filter) && app.tab == Tab::Library {
        let chunks = Layout::vertical([Constraint::Length(1), Constraint::Min(1)]).split(area);
        let filter_line = Line::from(vec![
            Span::styled("filter: ", Style::default().fg(CYAN)),
            Span::styled(&app.library_filter, Style::default().fg(Color::White)),
            Span::styled("_", Style::default().fg(CYAN)),
        ]);
        f.render_widget(Paragraph::new(filter_line), chunks[0]);
        chunks[1]
    } else {
        area
    };

    let filtered = app.filtered_library_items();
    if filtered.is_empty() {
        let msg = Paragraph::new("  No cached files.").style(Style::default().fg(DIM));
        f.render_widget(msg, content_area);
        return;
    }

    let name_width = content_area.width.saturating_sub(38);
    let widths = [
        Constraint::Length(name_width.max(16)),
        Constraint::Length(16),
        Constraint::Length(10),
        Constraint::Length(12),
    ];
    let header = Row::new(vec!["Name", "Bucket", "Size", "Cached"])
        .style(Style::default().fg(CYAN).add_modifier(Modifier::BOLD));

    let visible_height = content_area.height.saturating_sub(1) as usize;
    let scroll = {
        let mut s = app.library_scroll;
        if app.library_selected < s {
            s = app.library_selected;
        }
        if app.library_selected >= s + visible_height {
            s = app.library_selected.saturating_sub(visible_height - 1);
        }
        s
    };

    let rows: Vec<Row> = filtered
        .iter()
        .enumerate()
        .skip(scroll)
        .take(visible_height)
        .map(|(i, file)| {
            let icon = "  ";
            let name_display = format!("{}{}", icon, file.name);
            let size_str = format_size(file.size as i64);
            let cached_str = format_cached_time(&file.cached_at);

            let row = Row::new(vec![
                ratatui::text::Text::from(name_display),
                ratatui::text::Text::from(file.bucket.clone()),
                ratatui::text::Text::from(size_str),
                ratatui::text::Text::from(cached_str),
            ]);

            if i == app.library_selected {
                row.style(Style::default().bg(SELECT_BG).add_modifier(Modifier::BOLD))
            } else {
                row
            }
        })
        .collect();

    let table = Table::new(rows, widths).header(header);
    f.render_widget(table, content_area);
}

fn format_cached_time(time: &std::time::SystemTime) -> String {
    let datetime: chrono::DateTime<chrono::Local> = (*time).into();
    datetime.format("%Y-%m-%d").to_string()
}

fn draw_rename(f: &mut Frame, app: &App) {
    let Mode::Rename { new_name, .. } = &app.mode else {
        return;
    };
    let area = fixed_centered_rect(50, 7, f.area());
    let inner = draw_overlay_block(f, "Rename", area);

    let lines = vec![
        Line::from(""),
        Line::from(vec![
            Span::styled("  New name: ", Style::default().fg(CYAN)),
            Span::styled(new_name.as_str(), Style::default().fg(Color::White)),
            Span::styled("_", Style::default().fg(CYAN)),
        ]),
        Line::from(""),
        Line::from(Span::styled(
            "  Press enter to save, esc to cancel",
            Style::default().fg(DIM),
        )),
    ];
    f.render_widget(Paragraph::new(lines), inner);
}

fn draw_library_info(f: &mut Frame, app: &App) {
    let Mode::LibraryInfo { file } = &app.mode else {
        return;
    };
    let area = fixed_centered_rect(60, 14, f.area());
    let inner = draw_overlay_block(f, "File Info", area);

    let field = |label: &str, value: &str| -> Line<'static> {
        Line::from(vec![
            Span::styled(format!("{:>12}  ", label), Style::default().fg(DIM)),
            Span::raw(value.to_string()),
        ])
    };

    let cached_str = format_cached_time(&file.cached_at);
    let lines = vec![
        Line::from(""),
        field("Name", &file.name),
        field("Bucket", &file.bucket),
        field("Key", &file.key),
        field(
            "Size",
            &format!("{} ({} B)", format_size(file.size as i64), file.size),
        ),
        field("Cached", &cached_str),
        field("Path", &file.path.to_string_lossy()),
    ];
    f.render_widget(Paragraph::new(lines), inner);
}

fn draw_tier_picker(f: &mut Frame, app: &App) {
    let Mode::TierPicker {
        key,
        storage_class: _,
        selected,
    } = &app.mode
    else {
        return;
    };
    let tiers = ["Standard", "Bulk", "Expedited"];
    let name = key.rsplit('/').next().unwrap_or(key);
    let h = (tiers.len() as u16 + 6).min(12);
    let area = fixed_centered_rect(45, h, f.area());
    let inner = draw_overlay_block(f, "Select Restore Tier", area);

    let chunks = Layout::vertical([
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Min(1),
    ])
    .split(inner);

    f.render_widget(
        Paragraph::new(Span::styled(
            format!("  {}", name),
            Style::default().fg(DIM),
        )),
        chunks[0],
    );

    let rows: Vec<Row> = tiers
        .iter()
        .enumerate()
        .map(|(i, tier)| {
            let row = Row::new(vec![format!("  {}", tier)]);
            if i == *selected {
                row.style(Style::default().bg(SELECT_BG).add_modifier(Modifier::BOLD))
            } else {
                row
            }
        })
        .collect();

    let table = Table::new(rows, [Constraint::Percentage(100)]);
    f.render_widget(table, chunks[2]);
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn storage_class_color(class: &str) -> Color {
    match class {
        "DEEP_ARCHIVE" => RED,
        "GLACIER" => ORANGE,
        "GLACIER_IR" => BLUE,
        "STANDARD" => GREEN,
        _ => DIM,
    }
}
