#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";

const repoRoot = process.cwd();
const frontendRoot = process.argv[2]
  ? path.resolve(repoRoot, process.argv[2])
  : path.join(repoRoot, "web/src");

const requireFromWeb = createRequire(path.join(repoRoot, "web/package.json"));
const ts = requireFromWeb("typescript");

const supportedMethods = new Set(["GET", "POST", "PUT", "PATCH", "DELETE"]);
const usedOperations = new Set();
let systemDeleteSourceFile = "";

scanDirectory(frontendRoot);

process.stdout.write(
  JSON.stringify({
    usedOperations: Array.from(usedOperations).sort(),
    systemDeleteHasConfirmName: systemDeleteSourceFile !== "",
    systemDeleteSourceFile,
  }),
);

function scanDirectory(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      scanDirectory(fullPath);
      continue;
    }
    if (!/\.(ts|tsx|js|jsx)$/.test(entry.name)) {
      continue;
    }
    scanFile(fullPath);
  }
}

function scanFile(filePath) {
  const sourceText = fs.readFileSync(filePath, "utf8");
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    scriptKind(filePath),
  );
  const bindings = collectApiBindings(sourceFile);
  if (bindings.direct.size === 0 && bindings.namespaces.size === 0) {
    return;
  }

  const visit = (node) => {
    if (ts.isCallExpression(node)) {
      const operation = readOperation(node.expression, bindings);
      if (operation) {
        const routePath = stringLiteral(node.arguments[0]);
        if (routePath) {
          usedOperations.add(`${operation.method} ${routePath}`);
          if (
            !systemDeleteSourceFile &&
            operation.method === "DELETE" &&
            routePath === "/systems/{system_id}" &&
            hasSystemDeleteConfirmName(node.arguments[1])
          ) {
            systemDeleteSourceFile = path.relative(repoRoot, filePath).replaceAll(path.sep, "/");
          }
        }
      }
    }
    ts.forEachChild(node, visit);
  };

  visit(sourceFile);
}

function scriptKind(filePath) {
  const ext = path.extname(filePath).toLowerCase();
  switch (ext) {
    case ".tsx":
      return ts.ScriptKind.TSX;
    case ".jsx":
      return ts.ScriptKind.JSX;
    case ".js":
      return ts.ScriptKind.JS;
    default:
      return ts.ScriptKind.TS;
  }
}

function collectApiBindings(sourceFile) {
  const direct = new Set();
  const namespaces = new Set();

  for (const statement of sourceFile.statements) {
    if (!ts.isImportDeclaration(statement)) {
      continue;
    }
    if (!ts.isStringLiteral(statement.moduleSpecifier)) {
      continue;
    }
    const moduleName = statement.moduleSpecifier.text;
    if (!isApiClientModule(moduleName)) {
      continue;
    }
    const importClause = statement.importClause;
    const bindings = importClause?.namedBindings;
    if (!bindings) {
      continue;
    }
    if (ts.isNamedImports(bindings)) {
      for (const element of bindings.elements) {
        const importedName = element.propertyName?.text ?? element.name.text;
        if (importedName === "api") {
          direct.add(element.name.text);
        }
      }
      continue;
    }
    if (ts.isNamespaceImport(bindings)) {
      namespaces.add(bindings.name.text);
    }
  }

  return { direct, namespaces };
}

function isApiClientModule(moduleName) {
  const normalized = moduleName.replace(/\.(js|jsx|ts|tsx)$/, "");
  return normalized === "@/lib/api/client" || /(^|\/)lib\/api\/client$/.test(normalized);
}

function readOperation(expression, bindings) {
  if (ts.isPropertyAccessExpression(expression)) {
    const method = expression.name.text.toUpperCase();
    if (!supportedMethods.has(method)) {
      return null;
    }
    if (isApiReceiver(expression.expression, bindings)) {
      return { method };
    }
  }

  if (ts.isElementAccessExpression(expression)) {
    const method = stringLiteral(expression.argumentExpression)?.toUpperCase();
    if (!method || !supportedMethods.has(method)) {
      return null;
    }
    if (isApiReceiver(expression.expression, bindings)) {
      return { method };
    }
  }

  return null;
}

function isApiReceiver(expression, bindings) {
  expression = unwrap(expression);
  if (ts.isIdentifier(expression)) {
    return bindings.direct.has(expression.text);
  }
  if (
    ts.isPropertyAccessExpression(expression) &&
    expression.name.text === "api" &&
    ts.isIdentifier(expression.expression)
  ) {
    return bindings.namespaces.has(expression.expression.text);
  }
  return false;
}

function hasSystemDeleteConfirmName(argument) {
  const params = getNestedObjectProperty(argument, ["params", "query"]);
  if (!params || !ts.isObjectLiteralExpression(params)) {
    return false;
  }
  return hasObjectProperty(params, "confirm_name");
}

function getNestedObjectProperty(expression, pathSegments) {
  let current = unwrap(expression);
  for (const segment of pathSegments) {
    if (!current || !ts.isObjectLiteralExpression(current)) {
      return undefined;
    }
    current = getObjectPropertyInitializer(current, segment);
  }
  return current;
}

function getObjectPropertyInitializer(objectLiteral, name) {
  for (const property of objectLiteral.properties) {
    if (!ts.isPropertyAssignment(property)) {
      continue;
    }
    if (propertyName(property.name) !== name) {
      continue;
    }
    return unwrap(property.initializer);
  }
  return undefined;
}

function hasObjectProperty(objectLiteral, name) {
  return objectLiteral.properties.some((property) => {
    if (ts.isPropertyAssignment(property)) {
      return propertyName(property.name) === name;
    }
    if (ts.isShorthandPropertyAssignment(property)) {
      return property.name.text === name;
    }
    return false;
  });
}

function propertyName(nameNode) {
  if (
    ts.isIdentifier(nameNode) ||
    ts.isStringLiteral(nameNode) ||
    ts.isNoSubstitutionTemplateLiteral(nameNode)
  ) {
    return nameNode.text;
  }
  return undefined;
}

function stringLiteral(expression) {
  const value = unwrap(expression);
  if (!value) {
    return undefined;
  }
  if (ts.isStringLiteral(value) || ts.isNoSubstitutionTemplateLiteral(value)) {
    return value.text;
  }
  return undefined;
}

function unwrap(expression) {
  let current = expression;
  while (current) {
    if (ts.isParenthesizedExpression(current) || ts.isAsExpression(current) || ts.isTypeAssertionExpression(current)) {
      current = current.expression;
      continue;
    }
    if (typeof ts.isSatisfiesExpression === "function" && ts.isSatisfiesExpression(current)) {
      current = current.expression;
      continue;
    }
    break;
  }
  return current;
}
