import { describe, it, expect, beforeAll } from 'vitest'
import fs from 'node:fs/promises'
import path from 'node:path'
import {
  configureGrammars,
  extractSymbols,
  extractSymbolsSync,
  supportedExt,
  kindRank,
  scoreSymbol,
  type Sym,
} from '../src/state/symbolIndex'

const wasmDir = path.resolve(import.meta.dirname, '../node_modules/tree-sitter-wasms/out')

beforeAll(() => {
  // Load grammar wasm bytes straight off disk; no bundler asset imports.
  configureGrammars({
    go: () => fs.readFile(path.join(wasmDir, 'tree-sitter-go.wasm')),
    typescript: () => fs.readFile(path.join(wasmDir, 'tree-sitter-typescript.wasm')),
    tsx: () => fs.readFile(path.join(wasmDir, 'tree-sitter-tsx.wasm')),
    javascript: () => fs.readFile(path.join(wasmDir, 'tree-sitter-javascript.wasm')),
    php: () => fs.readFile(path.join(wasmDir, 'tree-sitter-php.wasm')),
    python: () => fs.readFile(path.join(wasmDir, 'tree-sitter-python.wasm')),
    rust: () => fs.readFile(path.join(wasmDir, 'tree-sitter-rust.wasm')),
  })
})

const names = (syms: Sym[]) => syms.map((s) => `${s.kind}:${s.name}`)

describe('supportedExt', () => {
  it('accepts indexable extensions', () => {
    expect(supportedExt('a.go')).toBe(true)
    expect(supportedExt('a.ts')).toBe(true)
    expect(supportedExt('a.tsx')).toBe(true)
    expect(supportedExt('a.mjs')).toBe(true)
    expect(supportedExt('a')).toBe(false)
    expect(supportedExt('a.md')).toBe(false)
    expect(supportedExt('a.astro')).toBe(false)
    expect(supportedExt('a.css')).toBe(false)
    expect(supportedExt('a.test.ts')).toBe(true) // ext filter is by extension only
  })

  it('accepts Python sources/stubs and Rust sources, not similarly named files', () => {
    expect(supportedExt('src/app.py')).toBe(true)
    expect(supportedExt('src/app.pyi')).toBe(true)
    expect(supportedExt('src/lib.rs')).toBe(true)
    expect(supportedExt('app.pyc')).toBe(false)
    expect(supportedExt('app.py.bak')).toBe(false)
    expect(supportedExt('guide.rst')).toBe(false)
    expect(supportedExt('folder.py/README')).toBe(false)
  })
})

describe('Go extraction', () => {
  let src: string
  beforeAll(() => {
    src = `package main

import "fmt"

// Hello says hi.
func Hello(a string) error {
	return nil
}

func (s *Server) Start(ctx context.Context) error { return nil }

type Server struct {
	port int
}

type Handler interface {
	Handle()
}

type Alias = string

type (
	A int
	B struct{}
)

const MaxRetries = 3

const (
	AConst = 1
	BConst = 2
)

var cache = map[string]int{}
`
  })

  it('extracts functions, methods, types, vars, consts with 1-based positions', async () => {
    const syms = await extractSymbols('main.go', src)
    expect(names(syms)).toEqual([
      'function:Hello',
      'method:Start',
      'type:Server',
      'type:Handler',
      'type:Alias',
      'type:A',
      'type:B',
      'var:MaxRetries',
      'var:AConst',
      'var:BConst',
      'var:cache',
    ])
    expect(syms[0].line).toBe(6)
    expect(syms[0].col).toBe(6)
    expect(syms[1].line).toBe(10)
    expect(syms[1].col).toBe(18)
    expect(syms[2].line).toBe(12)
    // kind 'const' is folded into 'var'
  })

  it('works in sync mode once initialized', async () => {
    await extractSymbols('main.go', 'func Warm() {}')
    const syms = extractSymbolsSync('main.go', src)
    expect(names(syms)).toEqual([
      'function:Hello',
      'method:Start',
      'type:Server',
      'type:Handler',
      'type:Alias',
      'type:A',
      'type:B',
      'var:MaxRetries',
      'var:AConst',
      'var:BConst',
      'var:cache',
    ])
  })

  it('tolerates syntax errors and extracts partial symbols', async () => {
    const syms = await extractSymbols('broken.go', 'func Good() {}\nfunc Bad( {')
    expect(names(syms)).toContain('function:Good')
  })
})

describe('TypeScript extraction', () => {
  const src = `export function hi() {}
function local() {}
export const arrow = () => 1
const plain = 2
export let mutable = 0
class Cls {
  constructor() {}
  method() {}
  static create() {}
}
interface Iface { x: number }
type TAlias = string
enum Color { Red }
export default function def() {}
export class ECls {}
export interface EIface {}
`

  it('extracts functions, consts, classes, interfaces, types, enums, methods', async () => {
    const syms = await extractSymbols('a.ts', src)
    expect(names(syms)).toEqual([
      'function:hi',
      'function:local',
      'var:arrow',
      'var:plain',
      'var:mutable',
      'class:Cls',
      'method:constructor',
      'method:method',
      'method:create',
      'type:Iface',
      'type:TAlias',
      'type:Color',
      'function:def',
      'class:ECls',
      'type:EIface',
    ])
    expect(syms[0].line).toBe(1)
    expect(syms[2].col).toBe(14)
  })

  it('handles tsx files with generics and JSX', async () => {
    const src = `export default function Widget() { return <div/> }
const Helper = ({ a }: { a: string }) => <b>{a}</b>
const n = compute<number>(x)
interface Props { a: number }
`
    const syms = await extractSymbols('a.tsx', src)
    expect(names(syms)).toEqual([
      'function:Widget',
      'var:Helper',
      'var:n',
      'type:Props',
    ])
  })

  it('handles javascript files', async () => {
    const src = `export function hi() {}
class Foo {}
const arrow = () => 2
module.exports = { hi }
`
    const syms = await extractSymbols('a.js', src)
    expect(names(syms)).toEqual(['function:hi', 'class:Foo', 'var:arrow'])
  })
})

describe('PHP extraction', () => {
  const src = `<?php

const MAX = 10;

function hi($name) {
    return "hi $name";
}

class Router {
    public $routes = [];

    private const KIND = 'router';

    public function add(string $r): void {}

    public static function make(): Router {}
}

interface Handler {
    public function handle();
}

trait CacheAware {
    public function warm() {}
}

enum Status: string {
    case Ok = 'ok';
}
`
  ;[0, 1].forEach((useSync) => {
    it('extracts functions, classes, methods, interfaces, traits, enums, consts', async () => {
      let syms: Sym[]
      if (useSync) {
        await extractSymbols('warm.php', "<?php function Warm() {}")
        syms = extractSymbolsSync('app.php', src)
      } else {
        syms = await extractSymbols('app.php', src)
      }
      expect(names(syms)).toEqual([
        'var:MAX',
        'function:hi',
        'class:Router',
        'var:routes',
        'var:KIND',
        'method:add',
        'method:make',
        'type:Handler',
        'method:handle',
        'type:CacheAware',
        'method:warm',
        'type:Status',
      ])
      expect(syms.find((s) => s.name === 'MAX')!.line).toBe(3)
      expect(syms.find((s) => s.name === 'hi')!.line).toBe(5)
      expect(syms.find((s) => s.name === 'Router')!.line).toBe(9)
    })
  })

  it('tolerates syntax errors and extracts partial symbols', async () => {
    const syms = await extractSymbols('broken.php', '<?php function Good() {}\nfunction bad( {')
    expect(names(syms)).toContain('function:Good')
  })

  it('rejects non-php extensions', () => {
    expect(supportedExt('a.php')).toBe(true)
    expect(supportedExt('a.hh')).toBe(false)
  })
})

describe('Python extraction', () => {
  const src = `def greet(name):
    return name

@decorator
async def fetch():
    return None

class Service:
    def run(self):
        def helper():
            pass
        return helper()

    @classmethod
    def create(cls):
        return cls()

    @staticmethod
    async def ping():
        pass

def factory():
    class Local:
        def work(self):
            pass
    return Local
`

  it.each(['async', 'sync'])('extracts classes, methods and nested functions (%s)', async (mode) => {
    // Warm a different grammar so the sync path must switch languages too.
    await extractSymbols('warm.go', 'package main\nfunc Warm() {}')
    const syms = mode === 'sync'
      ? extractSymbolsSync('app.py', src)
      : await extractSymbols('app.py', src)
    expect(names(syms)).toEqual([
      'function:greet', 'function:fetch', 'class:Service', 'method:run',
      'function:helper', 'method:create', 'method:ping', 'function:factory',
      'class:Local', 'method:work',
    ])
    expect(syms.find((s) => s.name === 'fetch')).toEqual({ name: 'fetch', kind: 'function', line: 5, col: 11 })
    expect(syms.find((s) => s.name === 'Service')).toEqual({ name: 'Service', kind: 'class', line: 8, col: 7 })
    expect(syms.find((s) => s.name === 'run')).toEqual({ name: 'run', kind: 'method', line: 9, col: 9 })
    expect(syms.find((s) => s.name === 'helper')).toEqual({ name: 'helper', kind: 'function', line: 10, col: 13 })
  })

  it('indexes Python stub declarations', async () => {
    const syms = await extractSymbols('service.pyi', `def load(path: str) -> str: ...
class Service:
    def run(self) -> None: ...
`)
    expect(names(syms)).toEqual(['function:load', 'class:Service', 'method:run'])
  })

  it('keeps 1-based positions with Unicode, CRLF and tab indentation', async () => {
    const syms = await extractSymbols('unicode.py', '# 😀 café\r\nclass Café:\r\n\tdef méthode(self):\r\n\t\tpass\r\n')
    expect(syms).toEqual([
      { name: 'Café', kind: 'class', line: 2, col: 7 },
      { name: 'méthode', kind: 'method', line: 3, col: 6 },
    ])
  })

  it('does not index comments, strings, lambdas or references as declarations', async () => {
    const syms = await extractSymbols('data.py', `# class Ghost: pass
text = "def fake(): pass"
callback = lambda value: value
Service().run()
`)
    expect(syms).toEqual([])
  })

  it('tolerates incomplete code and keeps valid declarations', async () => {
    const syms = await extractSymbols('broken.py', 'def good():\n    pass\n\ndef broken(\n')
    expect(names(syms)).toContain('function:good')
  })
})

describe('Rust extraction', () => {
  const src = `pub struct Store<T> { value: T }
pub enum State { Ready, Waiting }
pub union Number { int: u32, float: f32 }
pub type Id = u64;
pub trait Run {
    type Output;
    const LIMIT: usize;
    fn run(&self) -> Self::Output;
    fn ready(&self) -> bool { true }
}
impl<T> Store<T> {
    pub fn new(value: T) -> Self { Self { value } }
    pub fn get(&self) -> &T {
        fn helper() {}
        &self.value
    }
}
impl Run for Store<u32> {
    type Output = u32;
    const LIMIT: usize = 3;
    fn run(&self) -> u32 { self.value }
}
pub const LIMIT: usize = 10;
static mut CACHE: u32 = 0;
mod nested {
    pub async fn load() {}
}
extern "C" { fn foreign(); }
`

  it.each(['async', 'sync'])('extracts types, trait/impl methods and free functions (%s)', async (mode) => {
    await extractSymbols('warm.py', 'def warm(): pass')
    const syms = mode === 'sync'
      ? extractSymbolsSync('lib.rs', src)
      : await extractSymbols('lib.rs', src)
    expect(names(syms)).toEqual([
      'type:Store', 'type:State', 'type:Number', 'type:Id', 'type:Run',
      'type:Output', 'var:LIMIT', 'method:run', 'method:ready', 'method:new',
      'method:get', 'function:helper', 'type:Output', 'var:LIMIT', 'method:run',
      'var:LIMIT', 'var:CACHE', 'function:load', 'function:foreign',
    ])
    expect(syms[0]).toEqual({ name: 'Store', kind: 'type', line: 1, col: 12 })
    expect(syms.find((s) => s.name === 'run')).toEqual({ name: 'run', kind: 'method', line: 8, col: 8 })
    expect(syms.find((s) => s.name === 'helper')).toEqual({ name: 'helper', kind: 'function', line: 14, col: 12 })
    expect(syms.find((s) => s.name === 'load')).toEqual({ name: 'load', kind: 'function', line: 26, col: 18 })
  })

  it('keeps 1-based positions with Unicode, CRLF and raw identifiers', async () => {
    const syms = await extractSymbols('unicode.rs', '// 😀 café\r\nfn café() {}\r\nfn r#match() {}\r\n')
    expect(syms).toEqual([
      { name: 'café', kind: 'function', line: 2, col: 4 },
      { name: 'r#match', kind: 'function', line: 3, col: 4 },
    ])
  })

  it('keeps functions inside associated const/type initializers as free functions', async () => {
    const syms = await extractSymbols('nested.rs', `impl Run for Store {
    const LIMIT: usize = { fn limit() {} 3 };
    type Output = [u8; { fn length() {} 4 }];
    fn run(&self) {}
}
`)
    expect(names(syms)).toEqual([
      'var:LIMIT', 'function:limit', 'type:Output', 'function:length', 'method:run',
    ])
  })

  it('does not index comments, strings, locals or references as declarations', async () => {
    const syms = await extractSymbols('main.rs', `// struct Ghost {}
fn main() {
    let text = "fn fake() {}";
    let callback = |value| value;
    Store::new().run();
}
`)
    expect(names(syms)).toEqual(['function:main'])
  })

  it('tolerates incomplete code and keeps valid declarations', async () => {
    const syms = await extractSymbols('broken.rs', 'fn good() {}\nfn broken(')
    expect(names(syms)).toContain('function:good')
  })
})

describe('query scoring', () => {
  const sym = (name: string, kind: Sym['kind'] = 'function'): Sym => ({
    name,
    kind,
    line: 1,
    col: 1,
  })

  it('ranks kind-weighted: type/class > function > method > var', () => {
    expect(kindRank('class')).toBeLessThan(kindRank('type'))
    expect(kindRank('type')).toBeLessThan(kindRank('function'))
    expect(kindRank('function')).toBeLessThan(kindRank('method'))
    expect(kindRank('method')).toBeLessThan(kindRank('var'))
  })

  it('gives startswith bonus over substring', () => {
    const start = scoreSymbol(sym('Handler'), 'hand')
    const mid = scoreSymbol(sym('MyHandler'), 'hand')
    expect(start).toBeGreaterThan(mid)
  })

  it('returns -1 for non-matches', () => {
    expect(scoreSymbol(sym('Handler'), 'xyz')).toBe(-1)
  })

  it('empty term matches everything with base score', () => {
    expect(scoreSymbol(sym('Handler'), '')).toBeGreaterThanOrEqual(0)
  })

  it('is case-insensitive', () => {
    expect(scoreSymbol(sym('HelloWorld'), 'hellow')).toBeGreaterThan(0)
  })
})
