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
    function visit(node) {
      if (ts.isCallExpression(node)) {
        const targetNode = ts.isPropertyAccessExpression(node.expression) ? node.expression.name : node.expression;
        let symbol = checker.getSymbolAtLocation(targetNode);
        if (symbol && (symbol.flags & ts.SymbolFlags.Alias)) symbol = checker.getAliasedSymbol(symbol);
        const declaration = symbol?.valueDeclaration || symbol?.declarations?.[0];
        if (symbol && declaration) {
          const targetFile = declaration.getSourceFile();
          const targetPath = path.relative(root, targetFile.fileName).split(path.sep).join("/");
          if (!targetPath.startsWith("../")) {
            result.references.push({
              target_name: symbol.getName(),
              target_path: targetPath,
              edge_type: "calls",
              ...location(sourceFile, targetNode),
            });
          }
        }
      }
      ts.forEachChild(node, visit);
    }
    visit(sourceFile);
    output.files.push(result);
  }
}

output.files.sort((left, right) => left.path.localeCompare(right.path));
process.stdout.write(JSON.stringify(output));
