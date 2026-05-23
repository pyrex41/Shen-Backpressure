//! CLI for the multilang-paired demo (Rust implementation).
//!
//! Reads JSONL from stdin, emits JSON-per-line to stdout. Same I/O
//! shape as `go/main.go`, `ts/main.ts`, `py/main.py`.
//!
//! Build + run:
//!
//!   rustc -O main.rs -o /tmp/mp-rs
//!   /tmp/mp-rs < ../fixture-inputs.jsonl
//!
//! Or via the parity-check.sh harness, which handles this for you.
//!
//! ## No crate dependencies
//!
//! Per the W3.1 constraint ("keep the demo runnable on a stock machine
//! with Go + Node + Python3 installed; if Rust requires cargo + crates
//! at build time, demote it"), this binary uses ONLY `std`. The JSON
//! parser below is a handcrafted minimal subset that handles exactly
//! the input shape produced by `fixture-inputs.jsonl` — object/array
//! literals with string/number/boolean leaves and shallow nesting.
//! It is NOT a general-purpose JSON parser; production code should
//! use `serde_json`.
//!
//! ## hasBulkLine
//!
//! Calls the W2.2-generated `has_bulk_line(…)` helper from
//! `guards_gen.rs` directly. The Rs emitter handles the
//! `[[Sku Qty] | Rest] -> true where (>= Qty 5.0)` pattern correctly.

// The generated guards expose accessors not used by this minimal CLI
// (`customer()`, `min_subtotal()`, etc.). Silence the `dead_code`
// noise so `rustc -O main.rs` produces a clean compile.
#![allow(dead_code)]

// The generated `guards_gen.rs` declares `mod shenguard { … }` (without
// `pub`), so we can't import its contents via a normal `mod` + `use`
// pair from the parent. `include!` textually inlines the file before
// the visibility check is applied, which gets us the `shenguard`
// module at the crate root with its publicly-named API.
include!("guards_gen.rs");
use shenguard::{Cart, CartItem, CustomerId, DiscountEligible, Sku, has_bulk_line};

use std::io::{self, BufRead, Write};

#[derive(Debug)]
struct ItemInput {
    sku: String,
    qty: f64,
}

#[derive(Debug)]
struct Input {
    case_id: String,
    customer_id: String,
    items: Vec<ItemInput>,
    subtotal: f64,
    min_subtotal: f64,
    min_items: f64,
}

// --- Tiny JSON parser (subset) ---

struct Parser<'a> {
    src: &'a [u8],
    pos: usize,
}

impl<'a> Parser<'a> {
    fn new(src: &'a str) -> Self {
        Parser { src: src.as_bytes(), pos: 0 }
    }

    fn skip_ws(&mut self) {
        while self.pos < self.src.len() && (self.src[self.pos] as char).is_whitespace() {
            self.pos += 1;
        }
    }

    fn expect(&mut self, c: u8) -> Result<(), String> {
        self.skip_ws();
        if self.pos < self.src.len() && self.src[self.pos] == c {
            self.pos += 1;
            Ok(())
        } else {
            Err(format!("expected '{}' at pos {}", c as char, self.pos))
        }
    }

    fn peek(&mut self) -> Option<u8> {
        self.skip_ws();
        self.src.get(self.pos).copied()
    }

    fn parse_string(&mut self) -> Result<String, String> {
        self.expect(b'"')?;
        let start = self.pos;
        while self.pos < self.src.len() && self.src[self.pos] != b'"' {
            // No escape handling beyond bare \", \\: the fixture set
            // does not exercise them; a stricter parser is out of scope.
            if self.src[self.pos] == b'\\' {
                self.pos += 2;
                continue;
            }
            self.pos += 1;
        }
        if self.pos >= self.src.len() {
            return Err("unterminated string".to_string());
        }
        let raw = std::str::from_utf8(&self.src[start..self.pos])
            .map_err(|e| e.to_string())?
            .to_string();
        self.pos += 1; // consume closing "
        Ok(raw.replace("\\\"", "\"").replace("\\\\", "\\"))
    }

    fn parse_number(&mut self) -> Result<f64, String> {
        self.skip_ws();
        let start = self.pos;
        if self.pos < self.src.len() && self.src[self.pos] == b'-' {
            self.pos += 1;
        }
        while self.pos < self.src.len()
            && (self.src[self.pos].is_ascii_digit()
                || self.src[self.pos] == b'.'
                || self.src[self.pos] == b'e'
                || self.src[self.pos] == b'E'
                || self.src[self.pos] == b'+'
                || self.src[self.pos] == b'-')
        {
            self.pos += 1;
        }
        let s = std::str::from_utf8(&self.src[start..self.pos]).map_err(|e| e.to_string())?;
        s.parse::<f64>().map_err(|e| e.to_string())
    }

    fn skip_value(&mut self) -> Result<(), String> {
        self.skip_ws();
        match self.peek() {
            Some(b'"') => {
                self.parse_string()?;
            }
            Some(b'[') => {
                self.expect(b'[')?;
                loop {
                    self.skip_ws();
                    if self.peek() == Some(b']') {
                        self.expect(b']')?;
                        break;
                    }
                    self.skip_value()?;
                    self.skip_ws();
                    if self.peek() == Some(b',') {
                        self.expect(b',')?;
                    }
                }
            }
            Some(b'{') => {
                self.expect(b'{')?;
                loop {
                    self.skip_ws();
                    if self.peek() == Some(b'}') {
                        self.expect(b'}')?;
                        break;
                    }
                    self.parse_string()?;
                    self.expect(b':')?;
                    self.skip_value()?;
                    self.skip_ws();
                    if self.peek() == Some(b',') {
                        self.expect(b',')?;
                    }
                }
            }
            Some(b't') | Some(b'f') | Some(b'n') => {
                while self.pos < self.src.len() && self.src[self.pos].is_ascii_alphabetic() {
                    self.pos += 1;
                }
            }
            _ => {
                self.parse_number()?;
            }
        }
        Ok(())
    }
}

fn parse_input(s: &str) -> Result<Input, String> {
    let mut p = Parser::new(s);
    p.expect(b'{')?;
    let mut case_id = String::new();
    let mut customer_id = String::new();
    let mut items: Vec<ItemInput> = Vec::new();
    let mut subtotal = 0.0;
    let mut min_subtotal = 0.0;
    let mut min_items = 0.0;
    loop {
        p.skip_ws();
        if p.peek() == Some(b'}') {
            p.expect(b'}')?;
            break;
        }
        let key = p.parse_string()?;
        p.expect(b':')?;
        match key.as_str() {
            "caseId" => case_id = p.parse_string()?,
            "customerId" => customer_id = p.parse_string()?,
            "subtotal" => subtotal = p.parse_number()?,
            "minSubtotal" => min_subtotal = p.parse_number()?,
            "minItems" => min_items = p.parse_number()?,
            "items" => {
                p.expect(b'[')?;
                loop {
                    p.skip_ws();
                    if p.peek() == Some(b']') {
                        p.expect(b']')?;
                        break;
                    }
                    p.expect(b'{')?;
                    let mut isku = String::new();
                    let mut iqty = 0.0;
                    loop {
                        p.skip_ws();
                        if p.peek() == Some(b'}') {
                            p.expect(b'}')?;
                            break;
                        }
                        let k = p.parse_string()?;
                        p.expect(b':')?;
                        match k.as_str() {
                            "sku" => isku = p.parse_string()?,
                            "qty" => iqty = p.parse_number()?,
                            _ => p.skip_value()?,
                        }
                        p.skip_ws();
                        if p.peek() == Some(b',') {
                            p.expect(b',')?;
                        }
                    }
                    items.push(ItemInput { sku: isku, qty: iqty });
                    p.skip_ws();
                    if p.peek() == Some(b',') {
                        p.expect(b',')?;
                    }
                }
            }
            _ => p.skip_value()?,
        }
        p.skip_ws();
        if p.peek() == Some(b',') {
            p.expect(b',')?;
        }
    }
    Ok(Input { case_id, customer_id, items, subtotal, min_subtotal, min_items })
}

// --- JSON serialization (minimal, matches Go/TS/Py output byte-for-byte) ---

fn write_output(
    w: &mut impl Write,
    case_id: &str,
    ok: bool,
    outcome: &str,
    item_count: usize,
    has_bulk: bool,
) -> io::Result<()> {
    write!(
        w,
        "{{\"caseId\":\"{}\",\"ok\":{},\"outcome\":\"{}\",\"itemCount\":{},\"hasBulkLine\":{}}}\n",
        case_id, ok, outcome, item_count, has_bulk
    )
}

fn process_row(input: &Input, w: &mut impl Write) -> io::Result<()> {
    let item_count = input.items.len();

    let mut built_items: Vec<CartItem> = Vec::with_capacity(item_count);
    for it in &input.items {
        match CartItem::new(Sku::new(it.sku.clone()), it.qty) {
            Ok(ci) => built_items.push(ci),
            Err(_) => {
                return write_output(w, &input.case_id, false, "rejected-item-build", item_count, false);
            }
        }
    }

    let customer = CustomerId::new(input.customer_id.clone());
    let cart = match Cart::new(input.subtotal, customer, built_items) {
        Ok(c) => c,
        Err(_) => {
            return write_output(w, &input.case_id, false, "rejected-cart-build", item_count, false);
        }
    };

    let has_bulk = has_bulk_line(cart.items());

    match DiscountEligible::new(cart, item_count as f64, input.min_subtotal, input.min_items) {
        Ok(_) => write_output(w, &input.case_id, true, "eligible", item_count, has_bulk),
        Err(_) => {
            let outcome = if !(input.subtotal >= input.min_subtotal) {
                "rejected-subtotal"
            } else {
                "rejected-items"
            };
            write_output(w, &input.case_id, false, outcome, item_count, has_bulk)
        }
    }
}

fn main() -> io::Result<()> {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut handle = stdout.lock();
    for line in stdin.lock().lines() {
        let line = line?;
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }
        let parsed = match parse_input(trimmed) {
            Ok(p) => p,
            Err(e) => {
                eprintln!("rs: invalid input line: {}", e);
                std::process::exit(1);
            }
        };
        process_row(&parsed, &mut handle)?;
    }
    Ok(())
}
