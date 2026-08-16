use loomarr_image::{capabilities, run_generate, run_generate_with_avif_threads};

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
    if args.len() == 5 && args[..4] == ["generate", "--protocol", "1", "--benchmark-avif-threads"] {
        let Ok(threads) = args[4].parse::<usize>() else {
            eprintln!("benchmark AVIF threads must be 1..8");
            std::process::exit(2);
        };
        if !(1..=8).contains(&threads) {
            eprintln!("benchmark AVIF threads must be 1..8");
            std::process::exit(2);
        }
        if let Err(err) =
            run_generate_with_avif_threads(std::io::stdin(), std::io::stdout(), threads)
        {
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
