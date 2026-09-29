use dioxus::html::input_data::MouseButton;
use dioxus::prelude::*;

use crate::prefs;
use crate::ui::cx;

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum DrawerWidth {
    /// 320 px.
    Narrow,
    /// 380 px.
    #[default]
    Default,
    /// 420 px.
    Wide,
}

impl DrawerWidth {
    fn preset_px(self) -> f64 {
        match self {
            DrawerWidth::Narrow => 320.0,
            DrawerWidth::Default => 380.0,
            DrawerWidth::Wide => 420.0,
        }
    }
}

/// Bounds a dragged drawer width must stay within — narrow enough to still be readable, wide
/// enough to leave room for `d-split-main`, and capped well under a typical viewport so it can
/// never be dragged wider than the screen.
const MIN_DRAWER_PX: f64 = 260.0;
const MAX_DRAWER_PX: f64 = 720.0;

/// Reads `draft.drawer_width` (see [`prefs::DRAWER_WIDTH_KEY`]), falling back to `preset` for
/// anything unset, unparseable, non-finite, or outside [`MIN_DRAWER_PX`, `MAX_DRAWER_PX`] — the
/// same "never trust a stored value further than it can hurt you" discipline
/// `prefs::is_hex_colour` applies to the theme/primary cookie.
fn saved_or_preset(preset: f64) -> f64 {
    prefs::get(prefs::DRAWER_WIDTH_KEY)
        .and_then(|s| s.parse::<f64>().ok())
        .filter(|w| w.is_finite() && (MIN_DRAWER_PX..=MAX_DRAWER_PX).contains(w))
        .unwrap_or(preset)
}

/// Master / detail (`d-split`). Below 1100 px the drawer becomes an overlay.
///
/// `drawer` is normally a [`Drawer`](super::Drawer); omit it for "nothing selected".
/// `flush` lets the split fill `main` edge to edge; `stack` gives the main column a uniform gap
/// between its blocks (Beacon's data-heavy pages) instead of per-block margins.
///
/// The drawer's width is drag-resizable from its left edge (`d-drawer-resize`) and persists
/// across the whole app (`prefs::DRAWER_WIDTH_KEY`) — one remembered width, not per-page, the same
/// way theme/primary are one remembered choice everywhere. `width` still sets the *starting*
/// width the first time a viewer ever opens a drawer, before they've dragged anything.
#[component]
pub fn Split(
    #[props(default)] width: DrawerWidth,
    #[props(default)] flush: bool,
    #[props(default)] stack: bool,
    drawer: Option<Element>,
    children: Element,
) -> Element {
    let class = cx(&[
        "d-split",
        match width {
            DrawerWidth::Narrow => "d-split--narrow",
            DrawerWidth::Default => "",
            DrawerWidth::Wide => "d-split--wide",
        },
        if flush { "d-split--flush" } else { "" },
    ]);
    let main = if stack { "d-split-main d-split-main--stack" } else { "d-split-main" };

    // drawer_px is only ever read to build the inline style below — it does not change which
    // preset class above is applied, so a still-undragged drawer looks identical to before this
    // was added (same class, same effective width, just also settable via style now).
    let mut drawer_px = use_signal(|| saved_or_preset(width.preset_px()));
    // Some(start_x, start_width) while a drag is in progress; None otherwise. Both are read again
    // from the *first* pointermove after pointerdown, not from pointerdown itself, so a plain
    // click that never moves never nudges drawer_px off its current value.
    let mut drag_from: Signal<Option<(f64, f64)>> = use_signal(|| None);

    let on_handle_down = move |ev: Event<PointerData>| {
        if ev.data().trigger_button().is_some_and(|b| b != MouseButton::Primary) {
            return;
        }
        let c = ev.data().client_coordinates();
        drag_from.set(Some((c.x, drawer_px())));
    };
    let on_move = move |ev: Event<PointerData>| {
        let Some((start_x, start_w)) = drag_from() else { return };
        let c = ev.data().client_coordinates();
        // The handle sits on the drawer's left edge: dragging it left (toward d-split-main, a
        // negative delta) makes the drawer wider, so width moves opposite to the cursor's own x.
        let next = (start_w + (start_x - c.x)).clamp(MIN_DRAWER_PX, MAX_DRAWER_PX);
        drawer_px.set(next);
    };
    let on_up = move |_: Event<PointerData>| {
        if drag_from().is_some() {
            drag_from.set(None);
            prefs::set(prefs::DRAWER_WIDTH_KEY, &drawer_px().round().to_string());
        }
    };

    let style = format!("--drawer:{}px", drawer_px());
    rsx! {
        div { class: "{class}", style: "{style}",
            section { class: "{main}", {children} }
            if let Some(d) = drawer {
                div {
                    class: "d-drawer-resize",
                    role: "separator",
                    aria_orientation: "vertical",
                    aria_label: "Resize drawer",
                    title: "Drag to resize",
                    onpointerdown: on_handle_down,
                }
                {d}
            }
            // A full-viewport capture layer, present only while dragging: without it, a fast drag
            // that outruns the handle's own (few-px-wide) bounds stops receiving pointermove the
            // instant the cursor leaves it, well before pointerup — confirmed necessary, not just
            // defensive, while building this against Beacon's traces drawer live.
            if drag_from().is_some() {
                div {
                    class: "d-resize-capture",
                    onpointermove: on_move,
                    onpointerup: on_up,
                    onpointercancel: on_up,
                }
            }
        }
    }
}
