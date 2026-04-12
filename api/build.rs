use std::path::PathBuf;

fn main() {
    let manifest_dir = std::env::var("CARGO_MANIFEST_DIR").unwrap();
    let proto_root = PathBuf::from(&manifest_dir);
    let out_dir = PathBuf::from(&manifest_dir).join("src/pb");

    // Create pb directory if it doesn't exist
    std::fs::create_dir_all(&out_dir).unwrap();

    let proto_files = vec![
        proto_root.join("core/registry/key_value/v1/service.proto"),
        proto_root.join("core/registry/key_value/v1/models.proto"),
    ];

    let mut prost_build = prost_build::Config::new();
    prost_build.out_dir(&out_dir);

    // Only compile with tonic for server feature (not WASM)
    #[cfg(feature = "server")]
    {
        tonic_build::configure()
            .compile_with_config(
                prost_build,
                &proto_files,
                &[proto_root.to_string_lossy().to_string()],
            )
            .unwrap();
    }

    // For WASM or when not using server feature, generate messages only
    #[cfg(not(feature = "server"))]
    {
        prost_build
            .compile_protos(
                &proto_files,
                &[proto_root.to_string_lossy().to_string()],
            )
            .unwrap();
    }

    // Generate dioxus-grpc hooks for web in OUT_DIR
    #[cfg(feature = "server")]
    {
        let out_dir_str = std::env::var("OUT_DIR").expect("OUT_DIR not set");
        let out_path = PathBuf::from(&out_dir_str);

        dioxus_grpc::generate_hooks(
            &proto_files,
            &[proto_root],
            &Some(&out_path),
            None,
            "http://localhost:50051",
        )
        .unwrap();
    }
}

