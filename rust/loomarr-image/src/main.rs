use loomarr_image::{capabilities, run_generate};

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args == ["generate", "--protocol", "1"] {
        if let Err(err) = run_generate(std::io::stdin(), std::io::stdout()) {
            eprintln!("{}: {}", err.code, err.message);
            std::process::exit(1);
        }
        println!();
        return;
    }
    if args != ["capabilities", "--protocol", "1", "--self-test"] {
        eprintln!("usage: loomarr-image <capabilities|generate> --protocol 1 [--self-test]");
        std::process::exit(2);
    }

    let release = option_env!("LOOMARR_RELEASE").unwrap_or("dev");
    match serde_json::to_writer(std::io::stdout(), &capabilities(release)) {
        Ok(()) => println!(),
        Err(err) => {
            eprintln!("serialize capabilities: {err}");
            std::process::exit(1);
        }
    }
}
