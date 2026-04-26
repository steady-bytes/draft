#[cfg(feature = "codegen")]
use {
    convert_case::{Case, Casing},
    std::{fmt::Write, path::Path},
    tonic_build::Config,
};

#[derive(Clone, Debug, PartialEq)]
pub struct GrpcConfig {
    pub host: String,
}

/// - to_path: Is the directory in which files should be written to. When [`None`], defaults to `OUT_DIR`
/// - prost_mod: If you moved the codegen of proto in a module
#[cfg(feature = "codegen")]
pub fn generate_hooks<P: AsRef<Path>, P2: AsRef<Path>, P3: AsRef<Path>>(
    protos: &[P],
    includes: &[P2],
    to_path: &Option<P3>,
    prost_mod: Option<&str>,
) -> Result<(), std::io::Error> {
    let mut config = Config::new();
    let file_descriptor_set = config.load_fds(protos, includes)?;

    for fd in file_descriptor_set.file {
        if fd.service.is_empty() {
            continue;
        }

        let pkg_name = fd
            .package
            .as_ref()
            .map_or_else(|| "_", |string| string.as_str());
        let filename = format!("{pkg_name}.dx.rs");
        let rust_pkg_name = pkg_name.replace('.', "_");

        let mut str = format!(
            "{mod_prost}\npub use proto::*;\nuse ::dioxus::prelude::*;\n",
            mod_prost = if let Some(mod_path) = prost_mod {
                format!("mod proto {{\n    pub use {mod_path}::{rust_pkg_name}::*;\n}}")
            } else {
                format!(
                    "#[path = \"{out_dir}/{pkg_name}.rs\"]\nmod proto;",
                    out_dir = std::env::var("OUT_DIR").expect("build.rs"),
                )
            },
        );


        for service in &fd.service {
            let tonic_client = format!(
                "proto::{}_client::{}Client",
                service.name().to_case(Case::Snake),
                service.name().to_case(Case::Pascal)
            );

            write!(
                str,
                "\npub struct {service_name}ServiceHook{tonic_client_ty};\n\npub fn use_{service_name_lowercase}_service() -> {service_name}ServiceHook {{\n    {service_name}ServiceHook{new_tonic_client}\n}}\n\nimpl {service_name}ServiceHook {{\n",
                service_name = service.name().to_case(Case::Pascal),
                service_name_lowercase = service.name().to_case(Case::Snake),
                tonic_client_ty = {
                    #[cfg(feature = "web")]
                    {
                        format!("({tonic_client}<::tonic_web_wasm_client::Client>)")
                    }
                    #[cfg(not(feature = "web"))]
                    {
                        format!("({tonic_client}<::tonic::transport::Channel>)")
                    }
                },
                new_tonic_client = {
                    #[cfg(feature = "web")]
                    {
                        format!("({{ let config = use_context::<::dioxus_grpc::GrpcConfig>(); {tonic_client}::new(::tonic_web_wasm_client::Client::new(config.host.clone())) }})")
                    }
                    #[cfg(not(feature = "web"))]
                    {
                        format!("({{ let config = use_context::<::dioxus_grpc::GrpcConfig>(); {tonic_client}::new(::tonic::transport::Endpoint::new(config.host.clone()).unwrap().connect_lazy()) }})")
                    }
                }
            )
            .expect("write error");

            for rpc in &service.method {
                if rpc.client_streaming() || rpc.server_streaming() {
                    continue;
                }
                write!(
                    str,
                    "    pub fn {rpc_name}(&self, req: Signal<{rpc_input}>) -> Resource<Result<{rpc_ouptut}, tonic::Status>> {{\n        let client = self.0.to_owned();\n        use_resource(move || {{\n            let mut client = client.clone();\n            async move {{ client.{rpc_name}(req()).await.map(|resp| resp.into_inner()) }}\n        }})\n    }}\n",
                    rpc_name = rpc.name().to_case(Case::Snake),
                    rpc_input = {
                        let mut full_path = rpc.input_type().split('.');
                        let ty = full_path.next_back().expect("build.rs");
                        let path = full_path.filter(|e| !e.is_empty()).collect::<Vec<_>>().join(".");

                        if path == pkg_name {
                            format!("proto::{ty}")
                        } else if let Some(mod_path) = prost_mod {
                            format!("{mod_path}::{path}::{ty}")
                        } else {
                            format!("super::{path}::{ty}")
                        }
                    },
                    rpc_ouptut = {
                        let mut full_path = rpc.output_type().split('.');
                        let ty = full_path.next_back().expect("build.rs");
                        let path = full_path.filter(|e| !e.is_empty()).collect::<Vec<_>>().join(".");

                        if path == pkg_name {
                            format!("proto::{ty}")
                        } else if let Some(mod_path) = prost_mod {
                            format!("{mod_path}::{path}::{ty}")
                        } else {
                            format!("super::{path}::{ty}")
                        }
                    }
                ).expect("write error");
            }

            str.push('}');
        }

        match to_path {
            Some(p) => {
                std::fs::write(
                    {
                        let mut path_to_file = p.as_ref().to_owned();
                        path_to_file.push(filename);
                        path_to_file
                    },
                    str,
                )
            },
            None => {
                std::fs::write(
                    format!("{}/{filename}", std::env::var("OUT_DIR").expect("build.rs")),
                    str,
                )
            },
        }?;
    }


    Ok(())
}
