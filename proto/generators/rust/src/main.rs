//! Writes the Rust the definitions generate: the messages by prost, the services by tonic.
//!
//! Usage: yoke-proto-generate <definitions root> <out dir> <file>...

use std::{env, path::PathBuf, process};

fn main() {
    let args: Vec<String> = env::args().skip(1).collect();
    if args.len() < 3 {
        eprintln!("usage: yoke-proto-generate <definitions root> <out dir> <file>...");
        process::exit(2);
    }
    let (root, out, files) = (PathBuf::from(&args[0]), PathBuf::from(&args[1]), &args[2..]);
    let set = protox::compile(files, [&root]).unwrap_or_else(|err| {
        eprintln!("the definitions do not compile: {err:?}");
        process::exit(1);
    });
    if let Err(err) = tonic_prost_build::configure().out_dir(&out).compile_fds(set) {
        eprintln!("the Rust cannot be generated: {err}");
        process::exit(1);
    }
}
