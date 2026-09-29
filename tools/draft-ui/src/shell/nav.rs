use crate::kinds::{AppKind, Tone};

/// One rail link.
#[derive(Clone, Debug, PartialEq)]
pub struct NavItem {
    pub label: String,
    /// An in-app path (`/gateway`) or, when `external`, a full URL.
    pub path: String,
    pub count: Option<u32>,
    /// A problem count (rendered in the error colour).
    pub count_err: bool,
    /// A link to another app: opens in a new tab and shows the ↗ marker.
    pub external: bool,
    /// The primary-coloured "+ New …" action.
    pub action: bool,
    /// A glyph before the label (the Apps block).
    pub glyph: Option<(String, Tone)>,
    /// Highlight only on an exact path match (default: also for sub-paths).
    pub exact: bool,
}

impl NavItem {
    pub fn new(label: impl Into<String>, path: impl Into<String>) -> Self {
        Self {
            label: label.into(),
            path: path.into(),
            count: None,
            count_err: false,
            external: false,
            action: false,
            glyph: None,
            exact: false,
        }
    }

    pub fn count(mut self, n: u32) -> Self {
        self.count = Some(n);
        self
    }

    pub fn count_err(mut self, n: u32) -> Self {
        self.count = Some(n);
        self.count_err = true;
        self
    }

    pub fn action(mut self) -> Self {
        self.action = true;
        self
    }

    pub fn exact(mut self) -> Self {
        self.exact = true;
        self
    }

    /// A link to another Draft app, with its glyph.
    pub fn app(app: AppKind, label: impl Into<String>, url: impl Into<String>) -> Self {
        Self {
            external: true,
            glyph: Some((app.code().to_string(), app.tone())),
            ..Self::new(label, url)
        }
    }

    /// Whether this item is the current page for `current` (a route path).
    pub fn is_current(&self, current: &str) -> bool {
        if self.external {
            return false;
        }
        let path = self.path.trim_end_matches('/');
        let cur = current.split(['?', '#']).next().unwrap_or(current).trim_end_matches('/');
        if path.is_empty() {
            // "/" matches only itself.
            return cur.is_empty();
        }
        cur == path || (!self.exact && cur.starts_with(&format!("{path}/")))
    }
}

/// A labelled group of rail links. A section with no items is not rendered.
#[derive(Clone, Debug, PartialEq)]
pub struct NavSection {
    pub label: String,
    pub items: Vec<NavItem>,
}

impl NavSection {
    pub fn new(label: impl Into<String>, items: Vec<NavItem>) -> Self {
        Self { label: label.into(), items }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn root_matches_only_itself() {
        let i = NavItem::new("Key/Value", "/");
        assert!(i.is_current("/"));
        assert!(!i.is_current("/gateway"));
    }

    #[test]
    fn sub_paths_highlight_their_section_unless_exact() {
        let g = NavItem::new("Gateway", "/gateway");
        assert!(g.is_current("/gateway"));
        assert!(g.is_current("/gateway/core-blueprint-ui"));
        assert!(!g.is_current("/gateways"));
        assert!(!NavItem::new("x", "/gateway").exact().is_current("/gateway/x"));
    }

    #[test]
    fn query_and_fragment_are_ignored() {
        assert!(NavItem::new("Q", "/query").is_current("/query?q=1#top"));
    }

    #[test]
    fn external_links_are_never_current() {
        assert!(!NavItem::app(AppKind::Beacon, "Beacon", "http://beacon.draft.localhost/").is_current("/"));
    }
}
