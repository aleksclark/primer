import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const contract = path.resolve(here, "../../build/openapi.json");
const outputDir = path.resolve(here, "src/main/kotlin/com/aleksclark/primertasks/generated");
const output = path.join(outputDir, "GeneratedTasksApi.kt");
const document = JSON.parse(await readFile(contract, "utf8"));

const KEYWORDS = new Set(["object", "class", "in", "out", "fun", "val", "var", "when", "package"]);

function fail(message) {
  throw new Error(message);
}

function refName(schema) {
  const name = schema?.$ref?.split("/")?.at(-1);
  if (!name) fail(`expected schema $ref, got ${JSON.stringify(schema)}`);
  return name;
}

function namedSchema(name) {
  const value = document.components?.schemas?.[name];
  if (!value) fail(`OpenAPI schema ${name} is required`);
  return value;
}

function unwrap(schema) {
  if (!schema || typeof schema !== "object") fail(`unsupported schema ${JSON.stringify(schema)}`);
  if (schema.$ref) return { schema, nullable: false };
  const types = Array.isArray(schema.type) ? schema.type : schema.type != null ? [schema.type] : [];
  const nullable = schema.nullable === true || types.includes("null");
  const filtered = types.filter((type) => type !== "null");
  const next = { ...schema };
  if (filtered.length === 1) next.type = filtered[0];
  else if (filtered.length === 0) delete next.type;
  else next.type = filtered;
  return { schema: next, nullable };
}

function isFreeFormObject(schema) {
  if (schema === true) return true;
  if (!schema || typeof schema !== "object") return false;
  if (schema.$ref || schema.properties && Object.keys(schema.properties).length) return false;
  if (schema.additionalProperties === true) return true;
  if (schema.additionalProperties && typeof schema.additionalProperties === "object") {
    const keys = Object.keys(schema.additionalProperties).filter((key) => key !== "description");
    return keys.length === 0 || (keys.length === 1 && schema.additionalProperties.type === "object");
  }
  return schema.type === "object" || schema.additionalProperties == null;
}

function isStringEnum(schema) {
  return Array.isArray(schema?.enum) && schema.enum.length > 0 && schema.enum.every((value) => typeof value === "string");
}

function kotlinType(schema) {
  if (schema.$ref) return refName(schema);
  const { schema: value, nullable } = unwrap(schema);
  if (value.$ref) return suffix(refName(value), nullable);
  if (value.oneOf || value.anyOf || value.allOf) {
    fail(`unsupported schema construct ${JSON.stringify(value)}`);
  }
  if (value.enum && !isStringEnum(value)) {
    fail(`unsupported enum schema ${JSON.stringify(value.enum)}`);
  }
  const types = Array.isArray(value.type) ? value.type : value.type != null ? [value.type] : [];
  if (types.includes("array")) {
    if (!value.items) fail("array schema is missing items");
    return suffix(`List<${kotlinType(value.items)}>`, nullable);
  }
  if (types.includes("object") || value.additionalProperties != null || value.properties) {
    if (value.properties && Object.keys(value.properties).length > 0) {
      fail(`inline object schemas are unsupported: ${JSON.stringify(Object.keys(value.properties))}`);
    }
    if (!isFreeFormObject(value)) fail(`untyped object is unsupported: ${JSON.stringify(value)}`);
    return suffix("JsonObject", nullable);
  }
  if (types.includes("integer")) {
    return suffix(value.format === "int32" ? "Int" : "Long", nullable);
  }
  if (types.includes("number")) return suffix("Double", nullable);
  if (types.includes("boolean")) return suffix("Boolean", nullable);
  if (types.includes("string") || types.length === 0) return suffix("String", nullable);
  fail(`unsupported schema type ${JSON.stringify(value)}`);
}

function suffix(type, nullable) {
  return nullable && !type.endsWith("?") ? `${type}?` : type;
}

function ident(name) {
  return KEYWORDS.has(name) ? `\`${name}\`` : name;
}

function toPascal(value) {
  return value.split(/[^A-Za-z0-9]+/).filter(Boolean).map((part) => part[0].toUpperCase() + part.slice(1)).join("");
}

function toCamel(value) {
  const pascal = toPascal(value);
  return pascal ? pascal[0].toLowerCase() + pascal.slice(1) : pascal;
}

function constName(operationId) {
  return operationId.replace(/[^A-Za-z0-9]+/g, "_").toUpperCase();
}

function jsonSchema(object) {
  return object?.content?.["application/json"]?.schema ?? null;
}

function successOf(operation) {
  const responses = operation.responses ?? {};
  for (const code of ["200", "201", "204"]) {
    if (responses[code]) return { code, schema: jsonSchema(responses[code]) };
  }
  return { code: null, schema: null };
}

function authKind(pathname, operationId) {
  if (pathname === "/health" || pathname === "/auth/login" || pathname === "/auth/callback") return "NONE";
  if (operationId === "device-pair" || operationId === "student-pair" || operationId === "management-device-enroll") return "NONE";
  if (pathname.startsWith("/student/") || pathname.startsWith("/device/")) return "DEVICE";
  if (pathname.startsWith("/management-device/")) return "MANAGEMENT";
  return "PARENT";
}

function enumIdent(value) {
  const identName = toPascal(String(value).replace(/[^A-Za-z0-9]+/g, "_"));
  if (!identName) fail(`cannot name enum value ${JSON.stringify(value)}`);
  return /^[A-Za-z]/.test(identName) ? identName : `Value${identName}`;
}

function enumSource(name, value) {
  if (!isStringEnum(value)) fail(`only string enums are supported for ${name}`);
  const entries = value.enum.map((entry) => `    @SerialName(${JSON.stringify(entry)}) ${enumIdent(entry)},`);
  return `@Serializable\nenum class ${name} {\n${entries.join("\n")}\n}`;
}

function modelSource(name) {
  const value = namedSchema(name);
  if (value.oneOf || value.anyOf || value.allOf) fail(`unsupported composition on ${name}`);
  if (isStringEnum(value)) return enumSource(name, value);
  const required = new Set(value.required ?? []);
  const fields = Object.entries(value.properties ?? {})
    .filter(([property]) => property !== "$schema")
    .map(([property, propertySchema]) => {
      let type = kotlinType(propertySchema);
      const optional = !required.has(property);
      if (optional && !type.endsWith("?")) type = `${type}?`;
      const fallback = optional || type.endsWith("?") ? " = null" : "";
      return `    val ${ident(property)}: ${type}${fallback},`;
    });
  return `@Serializable\ndata class ${name}(\n${fields.join("\n")}\n)`;
}

function queryTypeName(operationId) {
  return `${toPascal(operationId)}Query`;
}

function queryModel(operationId, parameters) {
  const fields = parameters.map((parameter) => {
    let type = kotlinType(parameter.schema ?? { type: "string" });
    if (!parameter.required && !type.endsWith("?")) type = `${type}?`;
    const fallback = parameter.required && !type.endsWith("?") ? "" : " = null";
    return `    val ${ident(parameter.name)}: ${type}${fallback},`;
  });
  return `@Serializable\ndata class ${queryTypeName(operationId)}(\n${fields.join("\n")}\n)`;
}

function queryPuts(operationId, parameters) {
  return parameters.map((parameter) => {
    const field = ident(parameter.name);
    return `        query.${field}?.let { httpUrl.addQueryParameter(${JSON.stringify(parameter.name)}, it.toString()) }`;
  }).join("\n");
}

function pathExpr(pathname, pathParams) {
  if (!pathParams.length) return JSON.stringify(pathname);
  return pathParams.reduce((expr, parameter) => `${expr}.replace("{${parameter.name}}", ${toCamel(parameter.name)})`, JSON.stringify(pathname));
}

const operations = [];
for (const [pathname, item] of Object.entries(document.paths ?? {})) {
  for (const [method, operation] of Object.entries(item ?? {})) {
    if (!operation?.operationId) continue;
    operations.push({ pathname, method: method.toUpperCase(), operation });
  }
}
operations.sort((a, b) => a.operation.operationId.localeCompare(b.operation.operationId));

const schemaNames = Object.keys(document.components?.schemas ?? {}).sort();
const queryTypes = [];
const methods = [];
const consts = [];

for (const entry of operations) {
  const operationId = entry.operation.operationId;
  const pathParams = (entry.operation.parameters ?? []).filter((parameter) => parameter.in === "path");
  const queryParams = (entry.operation.parameters ?? []).filter((parameter) => parameter.in === "query");
  const bodySchema = jsonSchema(entry.operation.requestBody);
  const success = successOf(entry.operation);
  if (queryParams.length) queryTypes.push(queryModel(operationId, queryParams));
  consts.push(`    const val ${constName(operationId)} = ${JSON.stringify(entry.pathname)}`);

  const args = [];
  for (const parameter of pathParams) args.push(`${toCamel(parameter.name)}: String`);
  if (queryParams.length) args.push(`query: ${queryTypeName(operationId)} = ${queryTypeName(operationId)}()`);
  if (bodySchema) args.push(`body: ${kotlinType(bodySchema)}`);
  args.push("token: String? = null");
  const returnType = success.schema ? kotlinType(success.schema) : "Unit";
  const bodyLine = bodySchema ? `json.encodeToString(${kotlinType(bodySchema)}.serializer(), body)` : "null";
  const queryBuild = queryParams.length ? `        val httpUrl = url(${pathExpr(entry.pathname, pathParams)})\n${queryPuts(operationId, queryParams)}\n        val requestUrl = httpUrl.build()` : `        val requestUrl = url(${pathExpr(entry.pathname, pathParams)}).build()`;
  const decode = returnType === "Unit"
    ? "        execute(method = " + JSON.stringify(entry.method) + ", url = requestUrl, body = " + bodyLine + ", auth = AuthKind." + authKind(entry.pathname, operationId) + ", token = token, expectBody = false)\n        return"
    : `        val payload = execute(method = ${JSON.stringify(entry.method)}, url = requestUrl, body = ${bodyLine}, auth = AuthKind.${authKind(entry.pathname, operationId)}, token = token, expectBody = true)\n        return json.decodeFromString(${returnType}.serializer(), payload)`;
  methods.push(`    fun ${toCamel(operationId)}(${args.join(", ")}): ${returnType} {
${queryBuild}
${decode}
    }`);
}

const source = `// Code generated from build/openapi.json by generate-client.mjs; DO NOT EDIT.
package com.aleksclark.primertasks.generated

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

object PrimerTasksOperations {
${consts.join("\n")}
}

${schemaNames.map(modelSource).join("\n\n")}

${queryTypes.join("\n\n")}

internal enum class AuthKind { NONE, PARENT, DEVICE, MANAGEMENT }

internal class GeneratedTasksApi(
    private val apiBaseUrl: String,
    private val http: OkHttpClient,
    private val json: Json,
    private val credentials: (AuthKind) -> String?,
) {
${methods.join("\n\n")}

    private fun url(path: String): HttpUrl.Builder = (apiBaseUrl + path).toHttpUrl().newBuilder()

    private fun execute(method: String, url: HttpUrl, body: String?, auth: AuthKind, token: String?, expectBody: Boolean): String {
        val builder = Request.Builder().url(url).method(method, requestBody(method, body))
        val resolved = token ?: if (auth == AuthKind.NONE) null else credentials(auth)
        if (auth != AuthKind.NONE) {
            if (resolved.isNullOrBlank()) throw TasksTransportException(401, "missing credential", "unauthorized")
            builder.header("Authorization", "Bearer " + resolved)
        }
        http.newCall(builder.build()).execute().use { response ->
            val payload = response.body?.string().orEmpty()
            if (!response.isSuccessful) throw decodeError(response.code, payload)
            if (!expectBody) return ""
            if (payload.isEmpty()) throw TasksTransportException(response.code, "empty response")
            return payload
        }
    }

    private fun requestBody(method: String, body: String?) = when {
        method == "GET" || method == "HEAD" -> null
        body != null -> body.toRequestBody(JSON)
        else -> ByteArray(0).toRequestBody(JSON)
    }

    private fun decodeError(status: Int, payload: String): TasksTransportException {
        return try {
            val problem = json.decodeFromString(Problem.serializer(), payload)
            TasksTransportException(status, problem.detail.ifBlank { problem.message }, problem.code, problem.detail)
        } catch (_: Exception) {
            TasksTransportException(status, payload.ifBlank { "request failed" })
        }
    }

    companion object {
        private val JSON = "application/json".toMediaType()
    }
}

class TasksTransportException(
    val statusCode: Int,
    message: String = "request failed",
    val code: String? = null,
    val detail: String? = null,
) : Exception(message)
`;

await mkdir(outputDir, { recursive: true });
await writeFile(output, source);
console.log(`generated ${path.relative(process.cwd(), output)}`);
