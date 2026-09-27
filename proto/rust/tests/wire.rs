//! A message the crate carries encodes to the bytes the definitions' Go encodes it to, and decodes back.

use prost::Message;
use yoke_proto::plugin::v1::{RegisterRequest, Surface};

const ENCODED: &str = "0a0761637175697265120761637175697265200132047275737442130a116465766963653a696e737472756d656e74";

fn request() -> RegisterRequest {
    RegisterRequest {
        plugin: "acquire".into(),
        unit: "acquire".into(),
        protocol: 1,
        language: "rust".into(),
        declared: Some(Surface { capabilities: vec!["device:instrument".into()], ..Default::default() }),
        ..Default::default()
    }
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

#[test]
fn a_registration_request_encodes_as_the_go_does_and_decodes_back() {
    let encoded = request().encode_to_vec();
    assert_eq!(hex(&encoded), ENCODED);
    assert_eq!(RegisterRequest::decode(encoded.as_slice()).unwrap(), request());
}
