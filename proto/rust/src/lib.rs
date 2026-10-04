//! The protocol definitions of Yoke, generated for Rust.

/// The plugin contract.
pub mod plugin {
    /// Version 1 of the plugin contract.
    pub mod v1 {
        include!("gen/yoke.plugin.v1.rs");
    }
}

/// The administrative contract.
pub mod administrative {
    /// Version 1 of the administrative contract.
    pub mod v1 {
        include!("gen/yoke.administrative.v1.rs");
    }
}

/// The interface contract.
pub mod interface {
    /// Version 1 of the interface contract.
    pub mod v1 {
        include!("gen/yoke.interface.v1.rs");
    }
}
