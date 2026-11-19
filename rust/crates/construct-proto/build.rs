use std::{env, error::Error, fmt::Write, fs, path::PathBuf};

/// Kept separate from message configuration so service traits and clients can be
/// added here without changing how protobuf messages are generated.
struct ConnectServiceGenerator;

impl prost_build::ServiceGenerator for ConnectServiceGenerator {
    fn generate(&mut self, service: prost_build::Service, buf: &mut String) {
        let full_name = format!("{}.{}", service.package, service.proto_name);
        writeln!(buf, "pub mod {} {{", service.name_to_module()).unwrap();
        writeln!(buf, "pub const SERVICE_NAME: &str = {full_name:?};").unwrap();
        writeln!(buf, "pub const METHODS: &[crate::MethodDescriptor] = &[").unwrap();
        for method in service.methods {
            let path = format!("/{full_name}/{}", method.proto_name);
            writeln!(
                buf,
                "crate::MethodDescriptor {{ name: {:?}, path: {:?}, input_type: {:?}, output_type: {:?}, server_streaming: {} }},",
                method.proto_name, path, method.input_proto_type.trim_start_matches('.'),
                method.output_proto_type.trim_start_matches('.'), method.server_streaming,
            ).unwrap();
        }
        writeln!(buf, "];\n}}").unwrap();
    }
}

trait ServiceModuleName {
    fn name_to_module(&self) -> String;
}

impl ServiceModuleName for prost_build::Service {
    fn name_to_module(&self) -> String {
        let mut name = String::new();
        for (i, ch) in self.proto_name.chars().enumerate() {
            if ch.is_uppercase() && i > 0 {
                name.push('_');
            }
            name.extend(ch.to_lowercase());
        }
        name
    }
}

fn main() -> Result<(), Box<dyn Error>> {
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR")?);
    let source = manifest.join("../../../api/def");
    let vendor = manifest.join("proto-vendor");
    let out = PathBuf::from(env::var("OUT_DIR")?);
    let descriptor = out.join("construct.bin");
    let mut protos = fs::read_dir(source.join("construct/v1"))?
        .map(|entry| entry.map(|entry| entry.path()))
        .collect::<Result<Vec<_>, _>>()?;
    protos.retain(|path| path.extension().is_some_and(|ext| ext == "proto"));
    protos.sort();
    println!("cargo:rerun-if-changed={}", source.display());
    println!("cargo:rerun-if-changed={}", vendor.display());

    prost_build::Config::new()
        .protoc_executable(protoc_bin_vendored::protoc_bin_path()?)
        .file_descriptor_set_path(&descriptor)
        .compile_well_known_types()
        .extern_path(".google.protobuf", "::pbjson_types")
        .disable_comments([".google.type.Decimal.value"])
        .service_generator(Box::new(ConnectServiceGenerator))
        .compile_protos(
            &protos,
            &[source, vendor, protoc_bin_vendored::include_path()?],
        )?;

    pbjson_build::Builder::new()
        .register_descriptors(&fs::read(descriptor)?)?
        .build(&[".construct.v1", ".google.type"])?;
    Ok(())
}
