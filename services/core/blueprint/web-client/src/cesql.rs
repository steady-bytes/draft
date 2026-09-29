//! A client-side evaluator for the CESQL subset the events query bar accepts.
//!
//! Catalyst's `Query` RPC takes a typed expression tree, not text, and the client has no way to
//! build one, so the bar's text is evaluated here against the events already fetched. The subset is
//! what the shared grammar's chips and Filter builder produce:
//!
//! ```text
//! expr    := or
//! or      := and (OR and)*
//! and     := unary (AND unary)*
//! unary   := NOT unary | primary
//! primary := '(' expr ')' | EXISTS field | field ( '=' | '!=' | '<>' ) value
//!          | field [NOT] LIKE 'pattern' | field [NOT] IN '(' value {',' value} ')'
//! ```
//!
//! Fields are `type`, `source`, `id`, `subject`, any other attribute by name, and `body.<path>` into
//! a JSON payload. Comparisons ignore case (as the old matcher did); `%` and `_` in LIKE patterns
//! are wildcards. A field an event does not have never matches `=`, `LIKE` or `IN`, and always
//! matches `!=`, `NOT LIKE` and `NOT IN`. Text that is not CESQL at all (`payment`) is a plain
//! substring search across the event.

/// What an event exposes to a filter.
pub trait Fields {
    /// The field's value as text, or `None` when the event does not have it.
    fn field(&self, name: &str) -> Option<String>;
    /// Every searchable value joined, for the plain-text fallback.
    fn haystack(&self) -> String;
}

#[derive(Clone, Debug, PartialEq)]
pub enum Expr {
    And(Box<Expr>, Box<Expr>),
    Or(Box<Expr>, Box<Expr>),
    Not(Box<Expr>),
    Eq { field: String, value: String, negated: bool },
    Like { field: String, pattern: String, negated: bool },
    In { field: String, values: Vec<String>, negated: bool },
    Exists(String),
}

/// A parsed query bar.
#[derive(Clone, Debug, PartialEq)]
pub enum Filter {
    /// Nothing typed: every event matches.
    All,
    Text(String),
    Expr(Expr),
}

impl Filter {
    /// Reads the bar's text. `Err` carries a message worth showing when the text was plainly meant
    /// as CESQL but does not parse.
    pub fn parse(src: &str) -> Result<Filter, String> {
        let src = src.trim();
        if src.is_empty() {
            return Ok(Filter::All);
        }
        match parse(src) {
            Ok(expr) => Ok(Filter::Expr(expr)),
            Err(message) if looks_like_cesql(src) => Err(message),
            Err(_) => Ok(Filter::Text(src.to_string())),
        }
    }

    pub fn matches(&self, event: &impl Fields) -> bool {
        match self {
            Filter::All => true,
            Filter::Text(t) => event.haystack().to_lowercase().contains(&t.to_lowercase()),
            Filter::Expr(e) => eval(e, event),
        }
    }
}

/// Whether text has the shape of a CESQL expression rather than a search word.
fn looks_like_cesql(src: &str) -> bool {
    let lower = format!(" {} ", src.to_lowercase());
    src.contains(['=', '(', ')', '\'', '"', '<', '>', '!'])
        || [" like ", " in ", " and ", " or ", " not ", " exists "].iter().any(|k| lower.contains(k))
}

// Tokens -----------------------------------------------------------------------------------------

#[derive(Clone, Debug, PartialEq)]
enum Tok {
    Word(String),
    Str(String),
    Eq,
    Neq,
    Open,
    Close,
    Comma,
}

fn tokenize(src: &str) -> Result<Vec<Tok>, String> {
    let chars: Vec<char> = src.chars().collect();
    let mut i = 0;
    let mut out = Vec::new();
    while i < chars.len() {
        let c = chars[i];
        match c {
            c if c.is_whitespace() => i += 1,
            '(' => {
                out.push(Tok::Open);
                i += 1;
            }
            ')' => {
                out.push(Tok::Close);
                i += 1;
            }
            ',' => {
                out.push(Tok::Comma);
                i += 1;
            }
            '=' => {
                out.push(Tok::Eq);
                i += 1;
            }
            '!' if chars.get(i + 1) == Some(&'=') => {
                out.push(Tok::Neq);
                i += 2;
            }
            '<' if chars.get(i + 1) == Some(&'>') => {
                out.push(Tok::Neq);
                i += 2;
            }
            '\'' | '"' => {
                let quote = c;
                let mut s = String::new();
                i += 1;
                loop {
                    match chars.get(i) {
                        None => return Err("A string is missing its closing quote".to_string()),
                        // A backslash takes the next character literally; a doubled quote is a quote.
                        Some('\\') if i + 1 < chars.len() => {
                            s.push(chars[i + 1]);
                            i += 2;
                        }
                        Some(&q) if q == quote && chars.get(i + 1) == Some(&quote) => {
                            s.push(quote);
                            i += 2;
                        }
                        Some(&q) if q == quote => {
                            i += 1;
                            break;
                        }
                        Some(&other) => {
                            s.push(other);
                            i += 1;
                        }
                    }
                }
                out.push(Tok::Str(s));
            }
            c if c.is_alphanumeric() || matches!(c, '_' | '.' | '-' | '/' | ':' | '*' | '%') => {
                let start = i;
                while i < chars.len() && (chars[i].is_alphanumeric() || matches!(chars[i], '_' | '.' | '-' | '/' | ':' | '*' | '%')) {
                    i += 1;
                }
                out.push(Tok::Word(chars[start..i].iter().collect()));
            }
            other => return Err(format!("Unexpected character '{other}'")),
        }
    }
    Ok(out)
}

// Parser -----------------------------------------------------------------------------------------

struct Parser {
    toks: Vec<Tok>,
    pos: usize,
}

impl Parser {
    fn peek(&self) -> Option<&Tok> {
        self.toks.get(self.pos)
    }

    fn next(&mut self) -> Option<Tok> {
        let t = self.toks.get(self.pos).cloned();
        self.pos += 1;
        t
    }

    fn eat_keyword(&mut self, kw: &str) -> bool {
        if matches!(self.peek(), Some(Tok::Word(w)) if w.eq_ignore_ascii_case(kw)) {
            self.pos += 1;
            true
        } else {
            false
        }
    }

    fn or(&mut self) -> Result<Expr, String> {
        let mut left = self.and()?;
        while self.eat_keyword("or") {
            left = Expr::Or(Box::new(left), Box::new(self.and()?));
        }
        Ok(left)
    }

    fn and(&mut self) -> Result<Expr, String> {
        let mut left = self.unary()?;
        while self.eat_keyword("and") {
            left = Expr::And(Box::new(left), Box::new(self.unary()?));
        }
        Ok(left)
    }

    fn unary(&mut self) -> Result<Expr, String> {
        if self.eat_keyword("not") {
            return Ok(Expr::Not(Box::new(self.unary()?)));
        }
        self.primary()
    }

    fn primary(&mut self) -> Result<Expr, String> {
        if self.peek() == Some(&Tok::Open) {
            self.pos += 1;
            let inner = self.or()?;
            return match self.next() {
                Some(Tok::Close) => Ok(inner),
                _ => Err("A ( is missing its )".to_string()),
            };
        }
        if self.eat_keyword("exists") {
            return Ok(Expr::Exists(self.field()?));
        }
        let field = self.field()?;
        let negated = self.eat_keyword("not");
        if self.eat_keyword("like") {
            return match self.next() {
                Some(Tok::Str(pattern)) => Ok(Expr::Like { field, pattern, negated }),
                _ => Err("LIKE needs a quoted pattern, e.g. type LIKE '%order%'".to_string()),
            };
        }
        if self.eat_keyword("in") {
            if self.next() != Some(Tok::Open) {
                return Err("IN needs a list in parentheses, e.g. type IN ('a', 'b')".to_string());
            }
            let mut values = vec![self.value()?];
            while self.peek() == Some(&Tok::Comma) {
                self.pos += 1;
                values.push(self.value()?);
            }
            return match self.next() {
                Some(Tok::Close) => Ok(Expr::In { field, values, negated }),
                _ => Err("The IN list is missing its )".to_string()),
            };
        }
        if negated {
            return Err("NOT must be followed by LIKE or IN here".to_string());
        }
        match self.next() {
            Some(Tok::Eq) => Ok(Expr::Eq { field, value: self.value()?, negated: false }),
            Some(Tok::Neq) => Ok(Expr::Eq { field, value: self.value()?, negated: true }),
            _ => Err(format!("Expected =, !=, LIKE or IN after {field}")),
        }
    }

    fn field(&mut self) -> Result<String, String> {
        match self.next() {
            Some(Tok::Word(w)) => Ok(w),
            _ => Err("Expected a field name".to_string()),
        }
    }

    fn value(&mut self) -> Result<String, String> {
        match self.next() {
            Some(Tok::Str(s)) | Some(Tok::Word(s)) => Ok(s),
            _ => Err("Expected a value".to_string()),
        }
    }
}

pub fn parse(src: &str) -> Result<Expr, String> {
    let toks = tokenize(src)?;
    let mut p = Parser { toks, pos: 0 };
    let expr = p.or()?;
    match p.peek() {
        None => Ok(expr),
        Some(_) => Err("Unexpected text after the expression".to_string()),
    }
}

// Evaluation -------------------------------------------------------------------------------------

/// `%` matches any run of characters, `_` exactly one; the whole text must match. Case-insensitive.
pub fn like(text: &str, pattern: &str) -> bool {
    let t: Vec<char> = text.to_lowercase().chars().collect();
    let p: Vec<char> = pattern.to_lowercase().chars().collect();
    // Two-row DP over (pattern position, text position).
    let mut prev = vec![false; t.len() + 1];
    prev[0] = true;
    for (i, &pc) in p.iter().enumerate() {
        let mut cur = vec![false; t.len() + 1];
        cur[0] = pc == '%' && prev[0];
        for j in 1..=t.len() {
            cur[j] = match pc {
                '%' => cur[j - 1] || prev[j],
                '_' => prev[j - 1],
                c => prev[j - 1] && c == t[j - 1],
            };
        }
        prev = cur;
        let _ = i;
    }
    prev[t.len()]
}

pub fn eval(expr: &Expr, event: &impl Fields) -> bool {
    match expr {
        Expr::And(a, b) => eval(a, event) && eval(b, event),
        Expr::Or(a, b) => eval(a, event) || eval(b, event),
        Expr::Not(a) => !eval(a, event),
        Expr::Exists(field) => event.field(field).is_some(),
        Expr::Eq { field, value, negated } => match event.field(field) {
            Some(v) => v.eq_ignore_ascii_case(value) != *negated,
            None => *negated,
        },
        Expr::Like { field, pattern, negated } => match event.field(field) {
            Some(v) => like(&v, pattern) != *negated,
            None => *negated,
        },
        Expr::In { field, values, negated } => match event.field(field) {
            Some(v) => values.iter().any(|x| x.eq_ignore_ascii_case(&v)) != *negated,
            None => *negated,
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    struct Ev(HashMap<&'static str, &'static str>);

    impl Fields for Ev {
        fn field(&self, name: &str) -> Option<String> {
            self.0.get(name).map(|v| v.to_string())
        }
        fn haystack(&self) -> String {
            self.0.values().copied().collect::<Vec<_>>().join(" ")
        }
    }

    fn ev(pairs: &[(&'static str, &'static str)]) -> Ev {
        Ev(pairs.iter().copied().collect())
    }

    fn order() -> Ev {
        ev(&[("type", "order.created"), ("source", "/services/shop"), ("subject", "o-17"), ("body.status", "failed"), ("body.user.id", "42")])
    }

    fn matches(src: &str, e: &Ev) -> bool {
        Filter::parse(src).unwrap().matches(e)
    }

    #[test]
    fn nothing_typed_matches_everything() {
        assert!(matches("", &order()));
        assert!(matches("   ", &order()));
    }

    #[test]
    fn equality_ignores_case_and_accepts_both_quote_styles() {
        assert!(matches("type = 'ORDER.created'", &order()));
        assert!(matches("type=\"order.created\"", &order()));
        assert!(!matches("type = 'order.deleted'", &order()));
        assert!(matches("type != 'order.deleted'", &order()));
        assert!(matches("type <> 'order.deleted'", &order()));
    }

    #[test]
    fn like_uses_percent_and_underscore() {
        assert!(matches("type LIKE '%order%'", &order()));
        assert!(matches("source LIKE '/services/%'", &order()));
        assert!(matches("subject LIKE 'o-__'", &order()));
        assert!(!matches("subject LIKE 'o-_'", &order()));
        assert!(matches("type NOT LIKE '%invoice%'", &order()));
        assert!(!matches("type NOT LIKE '%order%'", &order()));
    }

    #[test]
    fn in_lists_take_several_values() {
        assert!(matches("type IN ('a', 'order.created')", &order()));
        assert!(!matches("type IN ('a', 'b')", &order()));
        assert!(matches("type NOT IN ('a', 'b')", &order()));
    }

    #[test]
    fn exists_checks_the_field_is_present() {
        assert!(matches("EXISTS subject", &order()));
        assert!(!matches("EXISTS traceparent", &order()));
        assert!(matches("EXISTS body.user.id", &order()));
    }

    #[test]
    fn body_paths_are_addressed_by_dots() {
        assert!(matches("body.status = 'failed'", &order()));
        assert!(matches("body.user.id = 42", &order()));
        assert!(!matches("body.status = 'ok'", &order()));
    }

    #[test]
    fn a_missing_field_only_matches_the_negative_forms() {
        let e = ev(&[("type", "x")]);
        assert!(!matches("subject = 'a'", &e));
        assert!(!matches("subject LIKE '%'", &e));
        assert!(!matches("subject IN ('a')", &e));
        assert!(matches("subject != 'a'", &e));
        assert!(matches("subject NOT LIKE 'a'", &e));
        assert!(matches("subject NOT IN ('a')", &e));
    }

    #[test]
    fn and_binds_tighter_than_or() {
        let e = order();
        // a OR (b AND c): the AND pair fails but the OR's first arm holds.
        assert!(matches("type = 'order.created' OR source = 'x' AND subject = 'y'", &e));
        // (a OR b) AND c: c fails, so the whole thing fails.
        assert!(!matches("(type = 'order.created' OR source = 'x') AND subject = 'y'", &e));
    }

    #[test]
    fn not_negates_a_group() {
        assert!(matches("NOT (type = 'a' OR type = 'b')", &order()));
        assert!(!matches("NOT type = 'order.created'", &order()));
    }

    #[test]
    fn what_the_builder_and_chips_emit_parses() {
        // The grammar's own chips and the builder's fragments, combined the way `combine` does.
        for q in [
            "type LIKE '%order%'",
            "source LIKE '%shop%'",
            "EXISTS subject",
            "body.status = 'failed'",
            "(type LIKE '%order%') AND source = '/services/shop'",
            "(type = 'a' OR type = 'b') AND EXISTS subject",
            "type IN ('a', 'b') AND subject NOT LIKE 'x%'",
            r"subject = 'it\'s'",
        ] {
            assert!(Filter::parse(q).is_ok(), "{q} should parse");
        }
        assert!(matches(r"subject = 'o-17'", &order()));
    }

    #[test]
    fn an_escaped_quote_is_part_of_the_value() {
        let e = ev(&[("subject", "it's")]);
        assert!(matches(r"subject = 'it\'s'", &e));
        assert!(matches("subject = 'it''s'", &e));
    }

    #[test]
    fn broken_cesql_reports_an_error() {
        assert!(Filter::parse("type =").is_err());
        assert!(Filter::parse("type = 'a").is_err());
        assert!(Filter::parse("(type = 'a'").is_err());
        assert!(Filter::parse("type LIKE").is_err());
        assert!(Filter::parse("type IN 'a'").is_err());
    }

    #[test]
    fn plain_words_search_the_whole_event() {
        assert!(matches("shop", &order()));
        assert!(matches("FAILED", &order()));
        assert!(!matches("refund", &order()));
        assert_eq!(Filter::parse("refund"), Ok(Filter::Text("refund".into())));
    }

    #[test]
    fn like_handles_edge_patterns() {
        assert!(like("", ""));
        assert!(like("abc", "%"));
        assert!(like("abc", "a%c"));
        assert!(like("abc", "%b%"));
        assert!(!like("abc", "a%d"));
        assert!(!like("abc", ""));
        assert!(like("A.B", "a._"));
    }
}
