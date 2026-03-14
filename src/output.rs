/// CLI output formatting — TTY-aware, pipe-friendly.
use std::io::{self, IsTerminal, Write};

use crate::aws_ops::ObjectInfo;
use crate::helpers::{display_name, format_size};

pub fn is_tty() -> bool {
    io::stdout().is_terminal()
}

/// Storage class short label for CLI output.
fn class_label(class: &str) -> &str {
    match class {
        "STANDARD" => "STD",
        "STANDARD_IA" => "STD-IA",
        "ONEZONE_IA" => "OZ-IA",
        "GLACIER_IR" => "GL-IR",
        "GLACIER" => "GLACIER",
        "DEEP_ARCHIVE" => "DEEP",
        "INTELLIGENT_TIERING" => "INT-T",
        "REDUCED_REDUNDANCY" => "RR",
        other => other,
    }
}

/// List objects to stdout. Matches Go behavior:
/// - TTY short: one name per line
/// - TTY long: aligned columns (CLASS SIZE DATE KEY)
/// - Pipe short: bare keys
/// - Pipe long: tab-separated
pub fn list_objects(
    w: &mut impl Write,
    items: &[ObjectInfo],
    prefix: &str,
    long: bool,
    tty: bool,
) -> io::Result<()> {
    if tty && long {
        for item in items {
            if item.is_prefix {
                writeln!(
                    w,
                    "{:<8} {:>10}  {:>19}  {}",
                    "PRE",
                    "",
                    "",
                    display_name(&item.key, prefix)
                )?;
            } else {
                let date = if item.last_modified.len() >= 19 {
                    &item.last_modified[..19]
                } else {
                    &item.last_modified
                };
                writeln!(
                    w,
                    "{:<8} {:>10}  {:>19}  {}",
                    class_label(&item.storage_class),
                    format_size(item.size),
                    date,
                    display_name(&item.key, prefix)
                )?;
            }
        }
    } else if tty {
        for item in items {
            writeln!(w, "{}", display_name(&item.key, prefix))?;
        }
    } else if long {
        // Pipe + long: tab-separated
        for item in items {
            if item.is_prefix {
                writeln!(w, "{}\t-\tPREFIX\t-", item.key)?;
            } else {
                writeln!(
                    w,
                    "{}\t{}\t{}\t{}",
                    item.key, item.size, item.storage_class, item.last_modified
                )?;
            }
        }
    } else {
        // Pipe + short: bare keys
        for item in items {
            writeln!(w, "{}", item.key)?;
        }
    }
    Ok(())
}

/// Format stat output for a single object.
pub fn format_stat(w: &mut impl Write, obj: &ObjectInfo, tty: bool) -> io::Result<()> {
    if tty {
        writeln!(w, "{:>12}  {}", "Key:", obj.key)?;
        writeln!(
            w,
            "{:>12}  {} ({} B)",
            "Size:",
            format_size(obj.size),
            obj.size
        )?;
        writeln!(w, "{:>12}  {}", "Class:", obj.storage_class)?;
        writeln!(w, "{:>12}  {}", "Modified:", obj.last_modified)?;
        if !obj.content_type.is_empty() {
            writeln!(w, "{:>12}  {}", "Type:", obj.content_type)?;
        }
        if !obj.etag.is_empty() {
            writeln!(w, "{:>12}  {}", "ETag:", obj.etag)?;
        }
        if !obj.restore_status.is_empty() {
            writeln!(w, "{:>12}  {}", "Restore:", obj.restore_status)?;
        }
    } else {
        writeln!(w, "key={}", obj.key)?;
        writeln!(w, "size={}", obj.size)?;
        writeln!(w, "class={}", obj.storage_class)?;
        writeln!(w, "modified={}", obj.last_modified)?;
        if !obj.content_type.is_empty() {
            writeln!(w, "type={}", obj.content_type)?;
        }
        if !obj.etag.is_empty() {
            writeln!(w, "etag={}", obj.etag)?;
        }
        if !obj.restore_status.is_empty() {
            writeln!(w, "restore={}", obj.restore_status)?;
        }
    }
    Ok(())
}
