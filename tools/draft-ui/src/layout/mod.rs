//! Page structure: head, toolbar, master/detail split, drawer.

mod drawer;
mod page_head;
mod split;

pub use drawer::{Drawer, DrawerBlock};
pub use page_head::{PageHead, SectionTitle, Toolbar};
pub use split::{DrawerWidth, Split};
