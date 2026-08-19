import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const contract = path.resolve(here, "../../build/openapi.json");
const output = path.resolve(here, "src/main/kotlin/com/aleksclark/primertasks/generated/GeneratedTasksApi.kt");
const document = JSON.parse(await readFile(contract, "utf8"));

function schema(name) {
  const value = document.components?.schemas?.[name];
  if (!value) throw new Error(`OpenAPI schema ${name} is required`);
  return value;
}

function endpoint(operationId) {
  for (const [pathname, operations] of Object.entries(document.paths ?? {})) {
    for (const operation of Object.values(operations)) {
      if (operation?.operationId === operationId) return pathname;
    }
  }
  throw new Error(`OpenAPI operation ${operationId} is required`);
}

function kotlinType(value) {
  if (value.$ref) return value.$ref.split("/").at(-1);
  const types = Array.isArray(value.type) ? value.type : [value.type];
  if (types.includes("array")) return `List<${kotlinType(value.items)}>`;
  if (types.includes("integer")) return "Int";
  if (types.includes("number")) return "Double";
  if (types.includes("boolean")) return "Boolean";
  return "String";
}

function defaultValue(type) {
  if (type === "String") return ' = ""';
  if (type === "Int") return " = 0";
  if (type === "Double") return " = 0.0";
  if (type === "Boolean") return " = false";
  if (type.startsWith("List<")) return " = emptyList()";
  return "";
}

function model(name, source = name) {
  const value = schema(source);
  const required = new Set(value.required ?? []);
  const fields = Object.entries(value.properties ?? {}).filter(([property]) => property !== "$schema").map(([property, propertySchema]) => {
    const rawType = kotlinType(propertySchema);
    const nullable = propertySchema.nullable === true;
    const type = nullable ? `${rawType}?` : rawType;
    const fallback = nullable ? " = null" : required.has(property) ? "" : defaultValue(rawType);
    return `    val ${property}: ${type}${fallback},`;
  });
  return `@Serializable\ndata class ${name}(\n${fields.join("\n")}\n)`;
}

const source = `// Code generated from build/openapi.json by generate-client.mjs; DO NOT EDIT.
package com.aleksclark.primertasks.generated

import kotlinx.serialization.Serializable

object PrimerTasksOperations {
    const val DEVICE_PAIR = "${endpoint("device-pair")}"
    const val STUDENT_PROFILE = "${endpoint("student-profile")}"
    const val STUDENT_CHECKLIST = "${endpoint("student-checklist")}"
}

${model("PairCode")}

${model("DevicePairResponse", "DevicePair")}

${model("StudentProfile", "Student")}

${model("ChecklistResponse", "Checklist")}

${model("ChecklistItem")}
`;

await mkdir(path.dirname(output), { recursive: true });
await writeFile(output, source);
console.log(`generated ${path.relative(process.cwd(), output)}`);
