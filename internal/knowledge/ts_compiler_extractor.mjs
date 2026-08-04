import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const compilerPath = process.argv[2];
const root = path.resolve(process.argv[3]);
const request = JSON.parse(fs.readFileSync(0, "utf8"));
const ts = require(compilerPath);

const inferredOptions = {
  allowJs: true,
  checkJs: false,
  jsx: ts.JsxEmit.Preserve,
  module: ts.ModuleKind.ESNext,
  moduleResolution: ts.ModuleResolutionKind.Bundler ?? ts.ModuleResolutionKind.Node10,
  target: ts.ScriptTarget.ES2022,
  skipLibCheck: true,
  noEmit: true,
};
const requestedFiles = request.files.map((file) => path.resolve(root, file));
const parsedConfigs = new Map();
const configurationDiagnostics = [];

function withinRoot(candidate) {
  const relative = path.relative(root, candidate);
  return relative === "" || (!relative.startsWith("..") && !path.isAbsolute(relative));
}

function parseConfig(configPath) {
  if (parsedConfigs.has(configPath)) return parsedConfigs.get(configPath);
  const configFile = ts.readConfigFile(configPath, ts.sys.readFile);
  if (configFile.error) {
    const message = ts.flattenDiagnosticMessageText(configFile.error.messageText, "\n");
    configurationDiagnostics.push({ file: path.relative(root, configPath).split(path.sep).join("/"), message });
    parsedConfigs.set(configPath, null);
    return null;
  }
  const parsed = ts.parseJsonConfigFileContent(configFile.config, ts.sys, path.dirname(configPath));
  for (const error of parsed.errors) {
    configurationDiagnostics.push({
      file: path.relative(root, configPath).split(path.sep).join("/"),
      message: ts.flattenDiagnosticMessageText(error.messageText, "\n"),
    });
  }
  parsedConfigs.set(configPath, parsed);
  return parsed;
}

// Resolve each requested source against the nearest tsconfig that actually
// includes it. Vite and other project-reference layouts commonly keep an
// empty solution tsconfig beside tsconfig.app.json/tsconfig.node.json; using
// only <repo>/tsconfig.json silently discarded their module settings.
function configForFile(file) {
  let directory = path.dirname(file);
  while (withinRoot(directory)) {
    let names = [];
    try {
      names = fs.readdirSync(directory).filter((name) => /^tsconfig(?:\.[^/]+)?\.json$/.test(name));
    } catch {
      names = [];
    }
    names.sort((left, right) => {
      if (left === "tsconfig.json") return -1;
      if (right === "tsconfig.json") return 1;
      return left.localeCompare(right);
    });
    for (const name of names) {
      const configPath = path.join(directory, name);
      const parsed = parseConfig(configPath);
      if (!parsed) continue;
      if (parsed.fileNames.some((configured) => path.resolve(configured) === file)) {
        return { configPath, parsed };
      }
    }
    if (directory === root) break;
    const parent = path.dirname(directory);
    if (parent === directory) break;
    directory = parent;
  }
  return null;
}

const groups = new Map();
for (const file of requestedFiles) {
  const configured = configForFile(file);
  const key = configured?.configPath ?? "inferred";
  if (!groups.has(key)) {
    groups.set(key, {
      files: [],
      options: configured ? { ...configured.parsed.options, noEmit: true } : inferredOptions,
      configuration: configured ? path.relative(root, configured.configPath).split(path.sep).join("/") : "inferred",
    });
  }
  groups.get(key).files.push(file);
}

const output = {
  configuration: [...groups.values()].map((group) => group.configuration).sort().join(","),
  files: [],
  diagnostics: [...configurationDiagnostics],
};

function exported(node) {
  return Boolean(node.modifiers?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword || modifier.kind === ts.SyntaxKind.DefaultKeyword));
}

function location(sourceFile, node) {
  const startByte = node.getStart(sourceFile, false);
  const endByte = node.end;
  return {
    start_line: sourceFile.getLineAndCharacterOfPosition(startByte).line + 1,
    end_line: sourceFile.getLineAndCharacterOfPosition(endByte).line + 1,
    start_byte: startByte,
    end_byte: endByte,
  };
}

function signature(sourceFile, node) {
  const text = node.getText(sourceFile).replace(/\s+/g, " ").trim();
  return text.length > 500 ? `${text.slice(0, 497)}...` : text;
}

function pushSymbol(result, sourceFile, node, name, kind, parent = "", isExported = exported(node)) {
  if (!name) return;
  result.symbols.push({
    name,
    kind,
    parent,
    signature: signature(sourceFile, node),
    exported: isExported,
    ...location(sourceFile, node),
  });
}

for (const group of groups.values()) {
  const program = ts.createProgram({ rootNames: group.files, options: group.options });
  const checker = program.getTypeChecker();
  const groupFiles = new Set(group.files.map((file) => path.resolve(file)));

  for (const diagnostic of ts.getPreEmitDiagnostics(program)) {
    if (!diagnostic.file) continue;
    const rel = path.relative(root, diagnostic.file.fileName).split(path.sep).join("/");
    if (rel.startsWith("../")) continue;
    output.diagnostics.push({ file: rel, message: ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n") });
  }

  for (const sourceFile of program.getSourceFiles()) {
    const absolute = path.resolve(sourceFile.fileName);
    const rel = path.relative(root, absolute).split(path.sep).join("/");
    if (rel.startsWith("../") || !groupFiles.has(absolute)) continue;
    const result = { path: rel, language: "typescript", symbols: [], imports: [], references: [] };
    for (const statement of sourceFile.statements) {
      if (ts.isImportDeclaration(statement) || ts.isExportDeclaration(statement)) {
        const specifier = statement.moduleSpecifier;
        if (specifier && ts.isStringLiteralLike(specifier)) {
          const resolved = ts.resolveModuleName(specifier.text, absolute, group.options, ts.sys).resolvedModule;
          let resolvedPath = "";
          if (resolved) {
            const candidate = path.relative(root, resolved.resolvedFileName).split(path.sep).join("/").replace(/\.d\.ts$/, ".ts");
            if (!candidate.startsWith("../")) resolvedPath = candidate;
          }
          result.imports.push({ target: specifier.text, resolved_path: resolvedPath, ...location(sourceFile, statement) });
        }
      }
      if (ts.isFunctionDeclaration(statement)) pushSymbol(result, sourceFile, statement, statement.name?.text, "function");
      if (ts.isInterfaceDeclaration(statement)) pushSymbol(result, sourceFile, statement, statement.name.text, "interface");
      if (ts.isTypeAliasDeclaration(statement)) pushSymbol(result, sourceFile, statement, statement.name.text, "type");
      if (ts.isClassDeclaration(statement)) {
        const className = statement.name?.text || "default";
        pushSymbol(result, sourceFile, statement, className, "class");
        for (const member of statement.members) {
          if (ts.isMethodDeclaration(member) || ts.isGetAccessorDeclaration(member) || ts.isSetAccessorDeclaration(member)) {
            pushSymbol(result, sourceFile, member, member.name?.getText(sourceFile), "method", className, exported(statement));
          }
        }
      }
      if (ts.isVariableStatement(statement)) {
        for (const declaration of statement.declarationList.declarations) {
          if (!ts.isIdentifier(declaration.name)) continue;
          const functionLike = declaration.initializer && (ts.isArrowFunction(declaration.initializer) || ts.isFunctionExpression(declaration.initializer));
          const constant = (statement.declarationList.flags & ts.NodeFlags.Const) !== 0;
          pushSymbol(result, sourceFile, statement, declaration.name.text, functionLike ? "function" : constant ? "constant" : "variable");
        }
      }
    }
    function recordReference(targetNode, edgeType) {
      let symbol = checker.getSymbolAtLocation(targetNode);
      if (symbol && (symbol.flags & ts.SymbolFlags.Alias)) symbol = checker.getAliasedSymbol(symbol);
      const declaration = symbol?.valueDeclaration || symbol?.declarations?.[0];
      if (!symbol || !declaration) return;
      const targetFile = declaration.getSourceFile();
      const targetPath = path.relative(root, targetFile.fileName).split(path.sep).join("/");
      if (targetPath.startsWith("../")) return;
      // TypeScript names a default-exported symbol "default", which matches no
      // symbol this extractor recorded — those carry the declared name. Every
      // use of `export default function App` therefore resolved to a target
      // that did not exist, and App looked unreferenced in its own app.
      let targetName = symbol.getName();
      if (targetName === "default" && declaration.name && declaration.name.text) {
        targetName = declaration.name.text;
      }
      result.references.push({
        target_name: targetName,
        target_path: targetPath,
        edge_type: edgeType,
        ...location(sourceFile, targetNode),
      });
    }
    // A name in a declaration is where a symbol is defined, and an import or
    // export clause is plumbing that moves it between files. Neither is a use,
    // and counting them would make every exported symbol look referenced.
    function isDefinitionOrBinding(node) {
      const parent = node.parent;
      if (!parent) return true;
      if (parent.name === node && (
        ts.isFunctionDeclaration(parent) || ts.isClassDeclaration(parent) ||
        ts.isInterfaceDeclaration(parent) || ts.isTypeAliasDeclaration(parent) ||
        ts.isEnumDeclaration(parent) || ts.isEnumMember(parent) ||
        ts.isMethodDeclaration(parent) || ts.isMethodSignature(parent) ||
        ts.isPropertyDeclaration(parent) || ts.isPropertySignature(parent) ||
        ts.isVariableDeclaration(parent) || ts.isParameter(parent) ||
        ts.isBindingElement(parent) || ts.isModuleDeclaration(parent) ||
        ts.isPropertyAssignment(parent) || ts.isShorthandPropertyAssignment(parent)
      )) return true;
      if (ts.isImportSpecifier(parent) || ts.isImportClause(parent) ||
          ts.isNamespaceImport(parent) || ts.isExportSpecifier(parent) ||
          ts.isImportEqualsDeclaration(parent) || ts.isNamespaceExport(parent)) return true;
      // A label is not a symbol.
      if (ts.isLabeledStatement(parent) || ts.isBreakOrContinueStatement(parent)) return true;
      return false;
    }
    // Recorded once per identifier: a call expression and the general pass
    // would otherwise both claim the callee and double its inbound count,
    // which is what "most depended-on" is ranked by.
    const claimed = new Set();
    function claim(node, edgeType) {
      if (!node || claimed.has(node)) return;
      claimed.add(node);
      recordReference(node, edgeType);
    }
    function visit(node) {
      if (ts.isCallExpression(node)) {
        claim(ts.isPropertyAccessExpression(node.expression) ? node.expression.name : node.expression, "calls");
      }
      // Rendering a component uses it. Without this every component reached
      // only through JSX has no inbound edge at all, so a React application
      // reports its entire component tree — App included — as dead code.
      if (ts.isJsxSelfClosingElement(node) || ts.isJsxOpeningElement(node)) {
        const tag = node.tagName;
        // Lowercase tags are intrinsic DOM elements, not symbols in this repo.
        if (ts.isIdentifier(tag) && /^[A-Z]/.test(tag.text)) claim(tag, "references");
        else if (ts.isPropertyAccessExpression(tag)) claim(tag.name, "references");
      }
      // Every remaining resolved name is a use: reading a constant, naming a
      // type in an annotation, passing a function without calling it. Counting
      // only calls left 43 constants and 54 types looking unreferenced in a
      // codebase that reads them constantly.
      if (ts.isIdentifier(node) && !isDefinitionOrBinding(node)) claim(node, "references");
      ts.forEachChild(node, visit);
    }
    visit(sourceFile);
    output.files.push(result);
  }
}

output.files.sort((left, right) => left.path.localeCompare(right.path));
process.stdout.write(JSON.stringify(output));
