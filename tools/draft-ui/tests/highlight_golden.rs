//! Golden fixtures shared with the Go kit: `tests/fixtures/highlight.json`.
//!
//! Rust is the source of truth. `UPDATE_GOLDEN=1 cargo test --test highlight_golden` rewrites the
//! file; otherwise the test fails when the tokenizer's output no longer matches it. The Go test in
//! `highlight_test.go` loads the same file, so the two tokenizers cannot drift apart.

use draft_ui::data::{highlight, CodeLang};

const CASES: &[(&str, &str)] = &[
    ("json", r#"{"a": "b", "n": -1.5, "ok": true, "z": null}"#),
    ("json", "{\n  \"positions\": {\n    \"live:catalyst\": [168.0, 1080.0]\n  },\n  \"zoom\": 0.476\n}"),
    ("json", r#"{"k": "a\"b", "e": 1e-3}"#),
    ("json", "[1, 2, 3]"),
    ("json", r#"{"unterminated": "abc"#),
    ("yaml", "- name: query-name\n  uses: foundry://grpc-call@v1\n  with:\n    id: ${steps.a.body.id}\n    n: 5   # seconds"),
    ("yaml", "body: { name: { first_name: Ada } }"),
    ("yaml", r##"color: "#fff""##),
    ("yaml", "steps:\n  - name: a\n    uses: builtin://sleep\n    with: { for: 5s }  # wait"),
    ("yaml", "apiVersion: bench/v1\nkind: Workflow\ntrigger:\n  webhook:\n    slug: crud-e2e\n    secret_ref: blueprint://secrets/bench/x"),
    ("yaml", "enabled: true\nretries: 3\nratio: 0.5\nname: 'quoted # not a comment'"),
    ("http", "GET http://x/health\nx-bench-run: r-1\ntimeout: 2s"),
    ("http", "HTTP/1.1 503 Service Unavailable\nserver: envoy\ncontent-length: 91\n\nupstream connect error"),
    ("http", "HTTP/1.1 200 OK\nserver: envoy\n\nkey: not a header"),
    ("plain", "just some text\nwith lines"),
    ("", ""),
];

fn esc(s: &str) -> String {
    let mut out = String::from("\"");
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

fn build() -> String {
    let mut out = String::from("[\n");
    for (i, (lang, input)) in CASES.iter().enumerate() {
        let spans = highlight(CodeLang::from_name(lang), input);
        let tokens: Vec<String> = spans
            .iter()
            .map(|s| format!("[{}, {}]", esc(s.class.unwrap_or("")), esc(&s.text)))
            .collect();
        out.push_str(&format!(
            "  {{\"lang\": {}, \"input\": {}, \"tokens\": [{}]}}{}\n",
            esc(lang),
            esc(input),
            tokens.join(", "),
            if i + 1 < CASES.len() { "," } else { "" }
        ));
    }
    out.push_str("]\n");
    out
}

#[test]
fn golden_matches() {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures/highlight.json");
    let built = build();
    if std::env::var("UPDATE_GOLDEN").is_ok() {
        std::fs::write(&path, &built).unwrap();
        return;
    }
    let current = std::fs::read_to_string(&path).unwrap_or_default();
    assert_eq!(
        current, built,
        "tests/fixtures/highlight.json is stale — run `UPDATE_GOLDEN=1 cargo test --test highlight_golden`"
    );
}
