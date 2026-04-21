use std::io::Result;

fn main() -> Result<()> {
    let protos_to_compile: Vec<_> = std::fs::read_dir("./proto/")?
        .flatten()
        .map(|entry| entry.path())
        .collect();

    // Compile protos
    tonic_build::configure().compile_protos(&protos_to_compile, &["./proto/"])?;

    // Add every file to mod.rs
    std::fs::write(
        format!(
            "{}/src/proto/mod.rs",
            std::env::current_dir()?.to_str().expect("build.rs"),
        ),
        protos_to_compile.iter().fold(String::new(), |acc, proto| {
            acc + &format!(
                "pub mod {};\n",
                proto
                    .to_str()
                    .expect("build.rs")
                    .split('/')
                    .next_back()
                    .expect("build.rs")
                    .replace(".proto", "")
            )
        }),
    )?;

    dioxus_grpc::generate_hooks(
        &protos_to_compile,
        &["./proto/"],
        &Some("./src/hook"),
        Some("crate::proto"),
        "http://127.0.0.1:50051",
    )?;

    // Move files to the correct place
    for proto in protos_to_compile {
        let file_name = proto
            .to_str()
            .expect("build.rs")
            .split('/')
            .next_back()
            .expect("build.rs")
            .replace(".proto", ".rs");

        let mut proto_content = String::from("#![allow(warnings, unused, deny)]\n");
        println!("{proto:#?}");
        proto_content.push_str(&std::fs::read_to_string(format!(
            "{}/{file_name}",
            std::env::var("OUT_DIR").expect("build.rs")
        ))?);

        std::fs::write(
            format!(
                "{}/src/proto/{file_name}",
                std::env::current_dir()?.to_str().expect("build.rs")
            ),
            proto_content,
        )?;
    }


    Ok(())
}
