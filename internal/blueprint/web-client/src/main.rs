use leptos::*;
use leptonic::prelude::*;

fn main() {
    mount_to_body(|| view! { <App/> })
}

#[component]
fn App() -> impl IntoView {
    let (count, set_count) = create_signal(0);

    view! {
        <Root default_theme=LeptonicTheme::default()>
            <AppBar style="z-index: 1; background: var(--brand-color); color: white;">
                <H3 style="margin-left: 1em; color: white;">"Blueprint"</H3>
            </AppBar> 

            <Drawer side=DrawerSide::Left shown=true style="padding: 0.5em; background-color: var(--brand-color); border-right: 1px solid gray;">
                <Stack spacing=Size::Em(0.6)>
                    <Button on_click=move |_| {} variant=ButtonVariant::Filled>"Home"</Button>
                    <Button on_click=move |_| {} variant=ButtonVariant::Filled>"Services"</Button>
                    <Button on_click=move |_| {} variant=ButtonVariant::Filled>"Key/Values"</Button>
                </Stack>
            </Drawer>
        </Root>
    }
}