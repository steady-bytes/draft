//! Syntax highlighting for `CodeBlock`: a small tokenizer for JSON, YAML (workflow snippets) and
//! HTTP messages. Pure and dependency-free. The Go kit ships an equivalent tokenizer; both are
//! checked against the same fixtures in `tests/fixtures/highlight/`.
//!
//! Token classes are the `tk-*` classes in `scss/components/_code.scss`.

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum CodeLang {
    #[default]
    Plain,
    Json,
    Yaml,
    Http,
}

impl CodeLang {
    pub fn from_name(name: &str) -> CodeLang {
        match name.to_ascii_lowercase().as_str() {
            "json" => CodeLang::Json,
            "yaml" | "yml" => CodeLang::Yaml,
            "http" => CodeLang::Http,
            _ => CodeLang::Plain,
        }
    }
}

/// One run of text and its token class (`None` = unstyled).
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Span {
    pub class: Option<&'static str>,
    pub text: String,
}

fn span(class: Option<&'static str>, text: impl Into<String>) -> Span {
    Span { class, text: text.into() }
}

pub fn highlight(lang: CodeLang, text: &str) -> Vec<Span> {
    let spans = match lang {
        CodeLang::Plain => vec![span(None, text)],
        CodeLang::Json => json(text),
        CodeLang::Yaml => yaml(text),
        CodeLang::Http => http(text),
    };
    merge(spans)
}

/// Joins adjacent spans with the same class so the DOM stays small.
fn merge(spans: Vec<Span>) -> Vec<Span> {
    let mut out: Vec<Span> = Vec::with_capacity(spans.len());
    for s in spans {
        if s.text.is_empty() {
            continue;
        }
        match out.last_mut() {
            Some(last) if last.class == s.class => last.text.push_str(&s.text),
            _ => out.push(s),
        }
    }
    out
}

// JSON --------------------------------------------------------------------------------------------------

fn json(text: &str) -> Vec<Span> {
    let chars: Vec<char> = text.chars().collect();
    let mut out = Vec::new();
    let mut i = 0;
    while i < chars.len() {
        let c = chars[i];
        if c == '"' {
            let start = i;
            i += 1;
            while i < chars.len() && chars[i] != '"' {
                if chars[i] == '\\' {
                    i += 1;
                }
                i += 1;
            }
            i = (i + 1).min(chars.len());
            let s: String = chars[start..i].iter().collect();
            // A string followed by `:` is an object key.
            let mut j = i;
            while j < chars.len() && chars[j].is_whitespace() {
                j += 1;
            }
            let is_key = chars.get(j) == Some(&':');
            out.push(span(Some(if is_key { "tk-key" } else { "tk-str" }), s));
        } else if c == '-' || c.is_ascii_digit() {
            let start = i;
            i += 1;
            while i < chars.len() && (chars[i].is_ascii_digit() || matches!(chars[i], '.' | 'e' | 'E' | '+' | '-')) {
                i += 1;
            }
            out.push(span(Some("tk-num"), chars[start..i].iter().collect::<String>()));
        } else if c.is_ascii_alphabetic() {
            let start = i;
            while i < chars.len() && chars[i].is_ascii_alphabetic() {
                i += 1;
            }
            let w: String = chars[start..i].iter().collect();
            let class = matches!(w.as_str(), "true" | "false" | "null").then_some("tk-val");
            out.push(span(class, w));
        } else {
            out.push(span(None, c.to_string()));
            i += 1;
        }
    }
    out
}

// YAML --------------------------------------------------------------------------------------------------

fn yaml(text: &str) -> Vec<Span> {
    let mut out = Vec::new();
    let lines: Vec<&str> = text.split('\n').collect();
    for (n, line) in lines.iter().enumerate() {
        yaml_line(line, &mut out);
        if n + 1 < lines.len() {
            out.push(span(None, "\n"));
        }
    }
    out
}

fn yaml_line(line: &str, out: &mut Vec<Span>) {
    // Comment (a `#` at the start or after whitespace, outside quotes).
    let mut comment_at = None;
    let mut quote: Option<char> = None;
    let mut prev_ws = true;
    for (idx, c) in line.char_indices() {
        match (quote, c) {
            (Some(q), c) if c == q => quote = None,
            (None, '"') | (None, '\'') => quote = Some(c),
            (None, '#') if prev_ws => {
                comment_at = Some(idx);
                break;
            }
            _ => {}
        }
        prev_ws = c.is_whitespace();
    }
    let (code, comment) = match comment_at {
        Some(i) => (&line[..i], Some(&line[i..])),
        None => (line, None),
    };

    // Leading indent and list dash.
    let trimmed = code.trim_start();
    let indent_len = code.len() - trimmed.len();
    out.push(span(None, &code[..indent_len]));
    let mut rest = trimmed;
    if let Some(after) = rest.strip_prefix("- ") {
        out.push(span(None, "- "));
        rest = after.trim_start_matches(' ');
    }

    // `key:` at the start of the remaining text.
    if let Some(colon) = find_key_colon(rest) {
        let key = &rest[..colon];
        out.push(span(Some("tk-key"), key));
        out.push(span(None, ":"));
        yaml_value(&rest[colon + 1..], out);
    } else {
        yaml_value(rest, out);
    }
    if let Some(c) = comment {
        out.push(span(Some("tk-com"), c));
    }
}

/// Index of the colon that ends a mapping key (`name:` followed by space or end), if the text
/// starts with one.
fn find_key_colon(s: &str) -> Option<usize> {
    let bytes = s.as_bytes();
    let mut i = 0;
    while i < bytes.len() {
        match bytes[i] {
            b':' if i > 0 && (i + 1 == bytes.len() || bytes[i + 1] == b' ') => return Some(i),
            b' ' | b'{' | b'[' | b'"' | b'\'' | b',' => return None,
            _ => i += 1,
        }
    }
    None
}

fn yaml_value(text: &str, out: &mut Vec<Span>) {
    let chars: Vec<char> = text.chars().collect();
    let mut word = String::new();
    let mut in_flow = 0usize;
    let mut i = 0;
    let flush = |word: &mut String, in_flow: usize, out: &mut Vec<Span>| {
        if !word.is_empty() {
            let w = std::mem::take(word);
            out.extend(classify_word(&w, in_flow > 0));
        }
    };
    while i < chars.len() {
        let c = chars[i];
        // `${steps.name.body.field}` — a placeholder is one token, braces included.
        if c == '$' && chars.get(i + 1) == Some(&'{') && word.is_empty() {
            let end = chars[i..].iter().position(|&x| x == '}').map_or(chars.len(), |p| i + p + 1);
            out.push(span(Some("tk-ph"), chars[i..end].iter().collect::<String>()));
            i = end;
            continue;
        }
        // A quoted string is one unstyled token.
        if (c == '"' || c == '\'') && word.is_empty() {
            let end = chars[i + 1..].iter().position(|&x| x == c).map_or(chars.len(), |p| i + 1 + p + 1);
            out.push(span(None, chars[i..end].iter().collect::<String>()));
            i = end;
            continue;
        }
        match c {
            ' ' | '\t' | ',' | '{' | '}' | '[' | ']' => {
                flush(&mut word, in_flow, out);
                if c == '{' || c == '[' {
                    in_flow += 1;
                } else if (c == '}' || c == ']') && in_flow > 0 {
                    in_flow -= 1;
                }
                out.push(span(None, c.to_string()));
            }
            c => word.push(c),
        }
        i += 1;
    }
    flush(&mut word, in_flow, out);
}

fn classify_word(w: &str, in_flow: bool) -> Vec<Span> {
    // `key:` inside a flow mapping.
    if in_flow {
        if let Some(k) = w.strip_suffix(':') {
            if !k.is_empty() {
                return vec![span(Some("tk-key"), k), span(None, ":")];
            }
        }
    }
    let class = if w.starts_with("${") {
        Some("tk-ph")
    } else if w.contains("://") {
        Some("tk-url")
    } else if w.parse::<f64>().is_ok() {
        Some("tk-num")
    } else if matches!(w, "true" | "false" | "null" | "yes" | "no" | "~") {
        Some("tk-val")
    } else {
        None
    };
    vec![span(class, w)]
}

// HTTP --------------------------------------------------------------------------------------------------

fn http(text: &str) -> Vec<Span> {
    let mut out = Vec::new();
    let lines: Vec<&str> = text.split('\n').collect();
    let mut in_body = false;
    for (n, line) in lines.iter().enumerate() {
        if n == 0 {
            if let Some(rest) = line.strip_prefix("HTTP/") {
                // Status line: highlight it as an error from 400 up.
                let code: u16 = rest.split_whitespace().nth(1).and_then(|c| c.parse().ok()).unwrap_or(0);
                out.push(span((code >= 400).then_some("tk-err"), *line));
            } else if let Some((method, rest)) = line.split_once(' ') {
                out.push(span(Some("tk-key"), method));
                out.push(span(None, format!(" {rest}")));
            } else {
                out.push(span(None, *line));
            }
        } else if in_body {
            out.push(span(None, *line));
        } else if line.trim().is_empty() {
            in_body = true;
            out.push(span(None, *line));
        } else if let Some((name, value)) = line.split_once(':') {
            out.push(span(Some("tk-key"), name));
            out.push(span(None, format!(":{value}")));
        } else {
            out.push(span(None, *line));
        }
        if n + 1 < lines.len() {
            out.push(span(None, "\n"));
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    type Tok = Vec<(Option<&'static str>, String)>;

    fn cls(lang: CodeLang, src: &str) -> Tok {
        highlight(lang, src).into_iter().map(|s| (s.class, s.text)).collect()
    }

    fn has(t: &Tok, class: &'static str, text: &str) -> bool {
        t.iter().any(|(c, x)| *c == Some(class) && x == text)
    }

    fn has_class(t: &Tok, class: &'static str) -> bool {
        t.iter().any(|(c, _)| *c == Some(class))
    }

    #[test]
    fn json_keys_strings_numbers_and_literals() {
        let c = cls(CodeLang::Json, r#"{"a": "b", "n": -1.5, "ok": true, "z": null}"#);
        assert!(has(&c, "tk-key", "\"a\""));
        assert!(has(&c, "tk-str", "\"b\""));
        assert!(has(&c, "tk-num", "-1.5"));
        assert!(has(&c, "tk-val", "true"));
        assert!(has(&c, "tk-val", "null"));
    }

    #[test]
    fn json_round_trips_text() {
        let src = "{\n  \"positions\": { \"x\": [1.0, 2] },\n  \"zoom\": 0.476\n}";
        let joined: String = highlight(CodeLang::Json, src).iter().map(|s| s.text.as_str()).collect();
        assert_eq!(joined, src);
    }

    #[test]
    fn json_escaped_quote_stays_inside_the_string() {
        let c = cls(CodeLang::Json, r#"{"k": "a\"b"}"#);
        assert!(has(&c, "tk-str", r#""a\"b""#));
    }

    #[test]
    fn yaml_keys_urls_placeholders_and_comments() {
        let src = "- name: query-name\n  uses: foundry://grpc-call@v1\n  with:\n    id: ${steps.a.body.id}\n    n: 5   # seconds";
        let c = cls(CodeLang::Yaml, src);
        assert!(has(&c, "tk-key", "name"));
        assert!(has(&c, "tk-url", "foundry://grpc-call@v1"));
        assert!(has(&c, "tk-ph", "${steps.a.body.id}"));
        assert!(has(&c, "tk-num", "5"));
        assert!(c.iter().any(|(k, t)| *k == Some("tk-com") && t.contains("seconds")));
    }

    #[test]
    fn yaml_flow_mapping_keys() {
        let c = cls(CodeLang::Yaml, "body: { name: { first_name: Ada } }");
        assert!(has(&c, "tk-key", "body"));
        assert!(has(&c, "tk-key", "name"));
        assert!(has(&c, "tk-key", "first_name"));
    }

    #[test]
    fn yaml_hash_inside_quotes_is_not_a_comment() {
        let c = cls(CodeLang::Yaml, r##"color: "#fff""##);
        assert!(!has_class(&c, "tk-com"));
    }

    #[test]
    fn yaml_round_trips_text() {
        let src = "steps:\n  - name: a\n    uses: builtin://sleep\n    with: { for: 5s }  # wait";
        let joined: String = highlight(CodeLang::Yaml, src).iter().map(|s| s.text.as_str()).collect();
        assert_eq!(joined, src);
    }

    #[test]
    fn http_request_and_error_status() {
        let req = cls(CodeLang::Http, "GET http://x/health\nx-bench-run: r-1");
        assert!(has(&req, "tk-key", "GET"));
        assert!(has(&req, "tk-key", "x-bench-run"));
        let res = cls(CodeLang::Http, "HTTP/1.1 503 Service Unavailable\nserver: envoy");
        assert!(res.iter().any(|(k, t)| *k == Some("tk-err") && t.starts_with("HTTP/1.1 503")));
        let ok = cls(CodeLang::Http, "HTTP/1.1 200 OK");
        assert!(!has_class(&ok, "tk-err"));
    }

    #[test]
    fn http_body_is_not_treated_as_headers() {
        let c = cls(CodeLang::Http, "HTTP/1.1 200 OK\nserver: envoy\n\nkey: not a header");
        assert_eq!(c.iter().filter(|(k, _)| *k == Some("tk-key")).count(), 1);
    }
}

