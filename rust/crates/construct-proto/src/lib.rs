//! Construct API messages and proto3 JSON implementations generated from `api/def`.

/// Static metadata for a Connect RPC method. Type names are fully qualified proto names.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct MethodDescriptor {
    pub name: &'static str,
    pub path: &'static str,
    pub input_type: &'static str,
    pub output_type: &'static str,
    pub server_streaming: bool,
}

#[allow(clippy::useless_borrows_in_formatting)]
pub mod google {
    pub mod r#type {
        include!(concat!(env!("OUT_DIR"), "/google.r#type.rs"));
        include!(concat!(env!("OUT_DIR"), "/google.r#type.serde.rs"));
    }
}

#[allow(clippy::useless_borrows_in_formatting)]
pub mod construct {
    pub mod v1 {
        include!(concat!(env!("OUT_DIR"), "/construct.v1.rs"));
        include!(concat!(env!("OUT_DIR"), "/construct.v1.serde.rs"));
    }
}
