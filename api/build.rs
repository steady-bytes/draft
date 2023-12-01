use std::path::Path;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    build_draft().unwrap();

    Ok(())
}

fn build_draft() -> Result<(), Box<dyn std::error::Error>> { 
  let out_dir = Path::new("./gen/rust/registry");
  let includes = Path::new("mod.rs");

  // configure the tonic builder
  tonic_build::configure()
      .out_dir(out_dir)
      .include_file(includes)
      .compile(&[
        "./registry/key_val/v1/service.proto",
      ], &["."])?;

  Ok(())
}