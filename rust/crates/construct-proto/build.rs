use prost::Message;
use std::{env, error::Error, fmt::Write, fs, path::PathBuf};

/// Connect-specific code generation, independent of message configuration.
struct ConnectServiceGenerator;

impl prost_build::ServiceGenerator for ConnectServiceGenerator {
    fn generate(&mut self, service: prost_build::Service, buf: &mut String) {
        let full_name = format!("{}.{}", service.package, service.proto_name);
        let module = service.name_to_module();
        let name = &service.name;
        writeln!(buf, "pub mod {module} {{").unwrap();
        writeln!(buf, "pub const SERVICE_NAME: &str = {full_name:?};").unwrap();
        writeln!(buf, "pub const METHODS: &[crate::MethodDescriptor] = &[").unwrap();
        for method in &service.methods {
            let path = format!("/{full_name}/{}", method.proto_name);
            writeln!(
                buf,
                "crate::MethodDescriptor {{ name: {:?}, path: {:?}, input_type: {:?}, output_type: {:?}, server_streaming: {} }},",
                method.proto_name, path, method.input_proto_type.trim_start_matches('.'),
                method.output_proto_type.trim_start_matches('.'), method.server_streaming,
            ).unwrap();
        }
        writeln!(buf, "];").unwrap();
        writeln!(
            buf,
            "#[construct_connect::async_trait] pub trait {name}: Send + Sync + 'static {{"
        )
        .unwrap();
        for method in &service.methods {
            let output = output_type(method);
            writeln!(buf, "async fn {}(&self, _req: construct_connect::Request<super::{}>) -> Result<construct_connect::Response<{output}>, construct_connect::ConnectError> {{ Err(construct_connect::ConnectError::new(construct_connect::Code::Unimplemented, {:?})) }}", method.name, method.input_type, format!("{} is not implemented", method.proto_name)).unwrap();
        }
        writeln!(buf, "}}").unwrap();
        writeln!(
            buf,
            "pub struct {name}Server<T: {name} + ?Sized> {{ inner: std::sync::Arc<T> }}"
        )
        .unwrap();
        writeln!(buf, "impl<T: {name}> {name}Server<T> {{ pub fn new(service: T) -> Self {{ Self {{ inner: std::sync::Arc::new(service) }} }} }}").unwrap();
        writeln!(buf, "impl<T: {name} + ?Sized> {name}Server<T> {{ pub fn from_arc(inner: std::sync::Arc<T>) -> Self {{ Self {{ inner }} }} pub fn router(&self) -> construct_connect::Router {{ let router = construct_connect::Router::new();").unwrap();
        for method in &service.methods {
            let handler = if method.server_streaming {
                "server_streaming"
            } else {
                "unary"
            };
            let path = format!("/{full_name}/{}", method.proto_name);
            writeln!(buf, "let service = self.inner.clone(); let router = router.route({path:?}, construct_connect::axum::routing::post(move |request: construct_connect::axum::extract::Request| {{ let service = service.clone(); async move {{ construct_connect::generated::{handler}(request, move |req| async move {{ service.{}(req).await }}).await }} }}));", method.name).unwrap();
        }
        writeln!(buf, "router }} }}").unwrap();
        writeln!(buf, "#[derive(Clone)] pub struct {name}Client {{ channel: construct_connect::Channel }} impl {name}Client {{ pub fn new(channel: construct_connect::Channel) -> Self {{ Self {{ channel }} }}").unwrap();
        for method in &service.methods {
            let output = output_type(method);
            let handler = if method.server_streaming {
                "server_streaming"
            } else {
                "unary"
            };
            let path = format!("/{full_name}/{}", method.proto_name);
            writeln!(buf, "pub async fn {}(&self, req: construct_connect::Request<super::{}>) -> Result<construct_connect::Response<{output}>, construct_connect::ConnectError> {{ self.channel.{handler}({path:?}, req).await }}", method.name, method.input_type).unwrap();
        }
        writeln!(
            buf,
            "}} }} pub use {module}::{{{name}, {name}Server, {name}Client}};"
        )
        .unwrap();
    }
}

fn output_type(method: &prost_build::Method) -> String {
    if method.server_streaming {
        format!(
            "construct_connect::Streaming<super::{}>",
            method.output_type
        )
    } else {
        format!("super::{}", method.output_type)
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
        .register_descriptors(&fs::read(&descriptor)?)?
        .build(&[".construct.v1", ".google.type"])?;
    let descriptors = prost_types::FileDescriptorSet::decode(fs::read(descriptor)?.as_slice())?;
    let path = out.join("construct.v1.serde.rs");
    let mut generated = fs::read_to_string(&path)?;
    for file in descriptors.file {
        if file.package.as_deref() != Some("construct.v1") {
            continue;
        }
        for message in file.message_type {
            let start = format!("impl serde::Serialize for {} {{", message.name());
            let Some(offset) = generated.find(&start) else {
                continue;
            };
            let end = offset
                + generated[offset..]
                    .find("impl<'de> serde::Deserialize")
                    .ok_or("missing generated deserializer")?;
            let mut implementation = generated[offset..end].to_owned();
            for field in message.field {
                if field.type_name.as_deref() == Some(".google.protobuf.Timestamp") {
                    let original =
                        format!("struct_ser.serialize_field(\"{}\", v)?;", field.json_name());
                    let replacement = format!(
                        "struct_ser.serialize_field(\"{}\", &crate::timestamp::Timestamp(v))?;",
                        field.json_name()
                    );
                    if !implementation.contains(&original) {
                        return Err(format!(
                            "unsupported Timestamp serialization: {}",
                            field.name()
                        )
                        .into());
                    }
                    implementation = implementation.replace(&original, &replacement);
                }
            }
            generated.replace_range(offset..end, &implementation);
        }
    }
    fs::write(path, generated)?;
    Ok(())
}
