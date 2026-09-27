//! The protocol definitions of Yoke, generated for Rust.

/// The plugin contract.
pub mod plugin {
    /// Version 1 of the plugin contract.
    pub mod v1 {
        include!("gen/yoke.plugin.v1.rs");
    }
}
