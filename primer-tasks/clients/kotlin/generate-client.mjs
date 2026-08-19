import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const contract = path.resolve(here, "../../build/openapi.yaml");
const output = path.resolve(here, "src/main/kotlin/com/aleksclark/primertasks/generated/GeneratedTasksApi.kt");
const document = JSON.parse(await readFile(contract, "utf8"));

function schema(name) {
  const value = document.components?.schemas?.[name];
  if (!value) throw new Error(`OpenAPI schema ${name} is required`);
  return value;
}

function endpoint(pathname, method) {
  if (!document.paths?.[pathname]?.[method]) throw new Error(`OpenAPI operation ${method.toUpperCase()} ${pathname} is required`);
  return pathname;
}

function kotlinType(value) {
  if (value.$ref) return value.$ref.split("/").at(-1);
  if (value.type === "array") return `List<${kotlinType(value.items)}>`;
  if (value.type === "integer") return "Int";
  if (value.type === "number") return "Double";
  if (value.type === "boolean") return "Boolean";
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
  const fields = Object.entries(value.properties ?? {}).map(([property, propertySchema]) => {
    const rawType = kotlinType(propertySchema);
    const nullable = propertySchema.nullable === true;
    const type = nullable ? `${rawType}?` : rawType;
    const fallback = nullable ? " = null" : required.has(property) ? "" : defaultValue(rawType);
    return `    val ${property}: ${type}${fallback},`;
  });
  return `@Serializable\ndata class ${name}(\n${fields.join("\n")}\n)`;
}

const source = `// Code generated from build/openapi.yaml by generate-client.mjs; DO NOT EDIT.
package com.aleksclark.primertasks.generated

import kotlinx.serialization.Serializable

object PrimerTasksOperations {
    const val DEVICE_PAIR = "${endpoint("/device/pair", "post")}"
    const val STUDENT_PROFILE = "${endpoint("/student/profile", "get")}"
    const val STUDENT_CHECKLIST = "${endpoint("/student/checklist", "get")}"
}

${model("PairCode", "Pair")}

${model("DevicePairResponse", "DevicePair")}

${model("StudentProfile", "Student")}

${model("ChecklistResponse", "Checklist")}

${model("ChecklistItem")}
`;

await mkdir(path.dirname(output), { recursive: true });
await writeFile(output, source);
console.log(`generated ${path.relative(process.cwd(), output)}`);
