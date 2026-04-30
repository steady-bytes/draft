use std::io::Result;

fn main() -> Result<()> {
    let current_dir = std::env::current_dir()?.to_str().expect("build.rs").to_string();
    let proto_out_dir = format!("{current_dir}/src/proto");
    let hook_out_dir = format!("{current_dir}/src/hook");

    let proto_dirs = [
        "./core/registry/key_value/v1/",
        "./core/registry/service_discovery/v1/",
        "./core/control_plane/networking/v1/",
    ];

    let protos_to_compile: Vec<_> = proto_dirs
        .iter()
        .flat_map(|dir| {
            std::fs::read_dir(dir)
                .unwrap_or_else(|_| panic!("could not read {dir}"))
                .flatten()
                .filter(|entry| {
                    entry.path().extension().map(|ext| ext == "proto").unwrap_or(false)
                })
                .filter(|entry| entry.path().is_file())
                .map(|entry| entry.path())
                .collect::<Vec<_>>()
        })
        .collect();

    tonic_prost_build::configure()
        .out_dir(&proto_out_dir)
        .build_server(false)
        .build_client(true)
        .build_transport(false)
        .compile_protos(&protos_to_compile, &[".".into()])?;

    // Generate proto/mod.rs by discovering tonic-build's actual output files.
    // tonic-build names outputs by proto package (e.g. core.registry.key_value.v1.rs), not by
    // source filename, so we discover what was generated rather than deriving from source names.
    // Module names replace dots with underscores since dots are invalid in Rust identifiers.
    let proto_mod_content = std::fs::read_dir(&proto_out_dir)?
        .flatten()
        .filter(|e| {
            let name = e.file_name().into_string().unwrap_or_default();
            name.ends_with(".rs") && name != "mod.rs"
        })
        .fold(String::new(), |acc, entry| {
            let name = entry.file_name().into_string().unwrap_or_default();
            let module_name = name.replace(".rs", "").replace('.', "_");
            acc + &format!("#[path = \"./{name}\"]\npub mod {module_name};\n")
        });
    std::fs::write(format!("{proto_out_dir}/mod.rs"), proto_mod_content)?;

    // Remove stale hook files before regenerating so deleted services don't linger.
    for entry in std::fs::read_dir(&hook_out_dir)?.flatten() {
        if entry.file_name().to_str().map(|n| n.ends_with(".dx.rs")).unwrap_or(false) {
            std::fs::remove_file(entry.path())?;
        }
    }

    dioxus_grpc::generate_hooks(
        &protos_to_compile,
        &["."],
        &Some(hook_out_dir.as_str()),
        Some("crate::proto"),
    )?;

    // Generate hook/mod.rs from the .dx.rs files written above.
    let hook_mod_content = std::fs::read_dir(&hook_out_dir)?
        .flatten()
        .filter(|e| e.file_name().to_str().map(|n| n.ends_with(".dx.rs")).unwrap_or(false))
        .fold(String::new(), |acc, entry| {
            let name = entry.file_name().into_string().unwrap_or_default();
            let module_name = name.replace(".dx.rs", "").replace('.', "_");
            acc + &format!("#[path = \"./{name}\"]\npub mod {module_name};\n")
        });
    std::fs::write(format!("{hook_out_dir}/mod.rs"), hook_mod_content)?;

    Ok(())
}
