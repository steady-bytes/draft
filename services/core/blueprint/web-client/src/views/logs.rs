use dioxus::prelude::*;

#[component]
fn LogRow(timestamp: String, level: String, message: String) -> Element {
    let level_class = match level.as_str() {
        "ERROR" => "badge badge-error",
        "WARN" => "badge badge-warning",
        "INFO" => "badge badge-info",
        "DEBUG" => "badge badge-ghost",
        _ => "badge",
    };

    rsx! {
        tr {
            td { class: "font-mono text-sm", "{timestamp}" }
            td {
                span { class: level_class, "{level}" }
            }
            td { "{message}" }
        }
    }
}

#[component]
pub fn Logs() -> Element {
    let logs = vec![
        ("2026-04-04 14:32:15.234", "INFO", "Service started successfully"),
        ("2026-04-04 14:32:16.567", "DEBUG", "Initializing database connection"),
        ("2026-04-04 14:32:17.891", "INFO", "Database connection established"),
        ("2026-04-04 14:32:18.123", "DEBUG", "Loading configuration from environment"),
        ("2026-04-04 14:32:19.456", "INFO", "Configuration loaded successfully"),
        ("2026-04-04 14:32:20.789", "WARN", "High memory usage detected: 78%"),
        ("2026-04-04 14:32:21.012", "DEBUG", "Cache initialized with 1000 entries"),
        ("2026-04-04 14:32:22.345", "INFO", "API server listening on port 8080"),
        ("2026-04-04 14:32:23.678", "DEBUG", "Health check endpoint ready"),
        ("2026-04-04 14:32:24.901", "INFO", "All services initialized"),
        ("2026-04-04 14:32:25.234", "ERROR", "Failed to connect to external service"),
        ("2026-04-04 14:32:26.567", "WARN", "Retrying connection attempt 1/3"),
    ];

    rsx! {
        div { class: "p-6",
            h1 { class: "text-3xl font-bold mb-6", "Logs" }
            div { class: "overflow-x-auto",
                table { class: "table table-zebra table-compact w-full",
                    thead { class: "bg-base-200",
                        tr {
                            th { "Timestamp" }
                            th { "Level" }
                            th { "Message" }
                        }
                    }
                    tbody {
                        {logs.into_iter().map(|(timestamp, level, message)| {
                            rsx! {
                                LogRow {
                                    timestamp: timestamp.to_string(),
                                    level: level.to_string(),
                                    message: message.to_string(),
                                }
                            }
                        })}
                    }
                }
            }
        }
    }
}
