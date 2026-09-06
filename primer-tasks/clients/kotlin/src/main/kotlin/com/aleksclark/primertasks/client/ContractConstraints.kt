package com.aleksclark.primertasks.client

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull

/**
 * Interpreter for Go-owned JSON Schema constraints used by Tasks contracts.
 * Unknown keys and unsupported constructs fail closed. JSON types are not coerced.
 */
object ContractConstraints {
    val implementedExtensions = setOf("x-maxBytes", "x-nonBlank", "x-equalFields", "x-uniqueNormalized")
    val metadataExtensions = setOf("x-manifest", "x-transport")
    private val allowedKeys = setOf(
        "type", "properties", "required", "additionalProperties", "items",
        "minLength", "maxLength", "minItems", "maxItems", "uniqueItems",
        "minimum", "maximum", "enum", "const", "format", "pattern",
        "description", "title", "nullable", "default", "example", "examples", "readOnly", "writeOnly",
        "\$ref", "\$schema", "\$id", "\$defs",
        "oneOf", "allOf", "not", "if", "then", "dependentRequired",
    ) + implementedExtensions + metadataExtensions

    fun requireSupported(schema: JsonObject, path: String = "\$", defs: Map<String, JsonObject> = emptyMap()) {
        for (key in schema.keys) {
            if (key !in allowedKeys) error("unsupported contract constraint $key at $path")
            if (key.startsWith("x-") && key !in implementedExtensions && key !in metadataExtensions) {
                error("unsupported contract constraint $key at $path")
            }
        }
        if (schema.containsKey("anyOf") || schema.containsKey("else") || schema.containsKey("exclusiveMinimum") ||
            schema.containsKey("exclusiveMaximum") || schema.containsKey("multipleOf")
        ) {
            error("unsupported contract construct at $path")
        }
        schema["\$ref"]?.jsonPrimitive?.contentOrNull?.let { ref ->
            resolve(ref, defs, path)
            return
        }
        typesOf(schema, path)
        schema["x-maxBytes"]?.let {
            check(hasType(schema, "string") && it.jsonPrimitive.let { p -> !p.isString && p.longOrNull != null && p.longOrNull!! > 0 }) {
                "x-maxBytes at $path must be a positive integer on a string schema"
            }
        }
        schema["x-nonBlank"]?.let {
            check(hasType(schema, "string") && booleanLiteral(it) == true) {
                "x-nonBlank at $path must be true on a string schema"
            }
        }
        schema["x-equalFields"]?.let {
            val fields = it.jsonArray.map { field -> field.jsonPrimitive.content }
            check((hasType(schema, "object") || schema.containsKey("properties")) && fields.size == 2 && fields.all { name -> name.isNotBlank() }) {
                "x-equalFields at $path must name exactly two object fields"
            }
        }
        schema["x-uniqueNormalized"]?.let {
            check(hasType(schema, "array") && booleanLiteral(it) == true) {
                "x-uniqueNormalized at $path must be true on an array schema"
            }
        }
        schema["properties"]?.jsonObject?.forEach { (name, child) ->
            requireSupported(child.jsonObject, "$path.properties.$name", defs)
        }
        schema["items"]?.let { requireSupported(it.jsonObject, "$path.items", defs) }
        listOf("oneOf", "allOf").forEach { key ->
            schema[key]?.jsonArray?.forEachIndexed { index, child ->
                requireSupported(child.jsonObject, "$path.$key[$index]", defs)
            }
        }
        schema["not"]?.let { requireSupported(it.jsonObject, "$path.not", defs) }
        schema["if"]?.let { requireSupported(it.jsonObject, "$path.if", defs) }
        schema["then"]?.let { requireSupported(it.jsonObject, "$path.then", defs) }
    }

    fun matches(schema: JsonObject, value: JsonElement, defs: Map<String, JsonObject> = emptyMap()): Boolean {
        return try {
            requireSupported(schema, defs = defs)
            matchesUnchecked(schema, value, defs)
        } catch (_: IllegalStateException) {
            false
        }
    }

    fun requireMatches(schema: JsonObject, value: JsonElement, defs: Map<String, JsonObject> = emptyMap()) {
        requireSupported(schema, defs = defs)
        check(matchesUnchecked(schema, value, defs)) { "payload failed contract constraints" }
    }

    private fun matchesUnchecked(schema: JsonObject, value: JsonElement, defs: Map<String, JsonObject>): Boolean {
        schema["\$ref"]?.jsonPrimitive?.contentOrNull?.let { ref ->
            return matchesUnchecked(resolve(ref, defs, "\$ref"), value, defs)
        }
        schema["allOf"]?.jsonArray?.forEach { part ->
            if (!matchesUnchecked(part.jsonObject, value, defs)) return false
        }
        schema["oneOf"]?.jsonArray?.let { alternatives ->
            if (alternatives.count { matchesUnchecked(it.jsonObject, value, defs) } != 1) return false
        }
        schema["not"]?.let { if (matchesUnchecked(it.jsonObject, value, defs)) return false }
        schema["const"]?.let { if (!jsonEqual(it, value)) return false }
        schema["enum"]?.jsonArray?.let { values ->
            if (values.none { jsonEqual(it, value) }) return false
        }
        schema["if"]?.let { condition ->
            if (matchesUnchecked(condition.jsonObject, value, defs)) {
                val thenSchema = schema["then"]?.jsonObject ?: return false
                if (!matchesUnchecked(thenSchema, value, defs)) return false
            }
        }
        val types = typesOf(schema, "type")
        val nullable = schema["nullable"]?.let { booleanLiteral(it) } == true || types.contains("null")
        if (value is JsonNull) return nullable || types.isEmpty()
        if (types.contains("object") && value !is JsonObject) return false
        if (types.contains("array") && value !is JsonArray) return false
        if (types.contains("boolean")) {
            if (booleanLiteral(value) == null) return false
        }
        if (types.contains("integer")) {
            val number = integerLiteral(value) ?: return false
            schema["minimum"]?.let { bound -> integerLiteral(bound)?.let { if (number < it) return false } }
            schema["maximum"]?.let { bound -> integerLiteral(bound)?.let { if (number > it) return false } }
        }
        if (types.contains("string")) {
            val text = stringLiteral(value) ?: return false
            val runes = text.codePointCount(0, text.length)
            schema["minLength"]?.let { bound -> integerLiteral(bound)?.let { if (runes < it) return false } }
            schema["maxLength"]?.let { bound -> integerLiteral(bound)?.let { if (runes > it) return false } }
            schema["x-maxBytes"]?.let { bound ->
                integerLiteral(bound)?.let { if (text.toByteArray(Charsets.UTF_8).size > it) return false }
            }
            if (booleanLiteral(schema["x-nonBlank"] ?: JsonNull) == true && text.trim().isEmpty()) return false
            schema["pattern"]?.jsonPrimitive?.contentOrNull?.let { pattern ->
                if (!Regex(pattern).matches(text)) return false
            }
            when (schema["format"]?.jsonPrimitive?.contentOrNull) {
                "uuid" -> if (!UUID.matches(text)) return false
                "date-time" -> if (!dateTime(text)) return false
                "uri" -> if (text.isBlank()) return false
                "int64", null -> Unit
                else -> return false
            }
        }
        if (types.contains("array") && value is JsonArray) {
            schema["minItems"]?.let { bound -> integerLiteral(bound)?.let { if (value.size < it) return false } }
            schema["maxItems"]?.let { bound -> integerLiteral(bound)?.let { if (value.size > it) return false } }
            schema["items"]?.jsonObject?.let { itemSchema ->
                if (value.any { !matchesUnchecked(itemSchema, it, defs) }) return false
            }
            if (booleanLiteral(schema["uniqueItems"] ?: JsonNull) == true && value.map { it.toString() }.toSet().size != value.size) {
                return false
            }
            if (booleanLiteral(schema["x-uniqueNormalized"] ?: JsonNull) == true) {
                val normalized = value.map { stringLiteral(it)?.trim()?.lowercase() ?: return false }
                if (normalized.toSet().size != normalized.size) return false
            }
        }
        if (value is JsonObject) {
            schema["required"]?.jsonArray?.forEach { key ->
                if (!value.containsKey(key.jsonPrimitive.content)) return false
            }
            val properties = schema["properties"]?.jsonObject
            val additional = schema["additionalProperties"]
            for ((key, child) in value) {
                val property = properties?.get(key)?.jsonObject
                if (property == null) {
                    if (booleanLiteral(additional ?: JsonNull) == false) return false
                } else if (!matchesUnchecked(property, child, defs)) {
                    return false
                }
            }
            schema["dependentRequired"]?.jsonObject?.forEach { (key, names) ->
                if (value.containsKey(key)) {
                    names.jsonArray.forEach { name ->
                        if (!value.containsKey(name.jsonPrimitive.content)) return false
                    }
                }
            }
            schema["x-equalFields"]?.jsonArray?.let { fields ->
                if (fields.size == 2 && !jsonEqual(value[fields[0].jsonPrimitive.content], value[fields[1].jsonPrimitive.content])) {
                    return false
                }
            }
        }
        return true
    }

    private fun resolve(ref: String, defs: Map<String, JsonObject>, path: String): JsonObject {
        val name = ref.removePrefix("#/components/schemas/")
        return defs[name] ?: error("unresolved \$ref $ref at $path")
    }

    private fun typesOf(schema: JsonObject, path: String): Set<String> {
        val type = schema["type"] ?: return emptySet()
        val names = when (type) {
            is JsonArray -> type.map { it.jsonPrimitive.content }
            is JsonPrimitive -> listOf(type.content)
            else -> error("unsupported type value at $path")
        }
        val allowed = setOf("object", "array", "string", "integer", "boolean", "null")
        check(names.all { it in allowed }) { "unsupported type ${names.joinToString()} at $path" }
        return names.toSet()
    }

    private fun hasType(schema: JsonObject, expected: String) = typesOf(schema, "type").contains(expected)

    private fun stringLiteral(value: JsonElement): String? {
        val primitive = value as? JsonPrimitive ?: return null
        if (!primitive.isString) return null
        return primitive.content
    }

    private fun booleanLiteral(value: JsonElement): Boolean? {
        val primitive = value as? JsonPrimitive ?: return null
        if (primitive.isString) return null
        return primitive.booleanOrNull
    }

    private fun integerLiteral(value: JsonElement): Long? {
        val primitive = value as? JsonPrimitive ?: return null
        if (primitive.isString) return null
        val number = primitive.longOrNull ?: return null
        return number
    }

    private fun jsonEqual(left: JsonElement?, right: JsonElement?): Boolean {
        if (left == null || right == null) return left == null && right == null
        if (left is JsonNull && right is JsonNull) return true
        if (left is JsonPrimitive && right is JsonPrimitive) {
            if (left.isString != right.isString) return false
            if (left.isString) return left.content == right.content
            booleanLiteral(left)?.let { return it == booleanLiteral(right) }
            integerLiteral(left)?.let { return it == integerLiteral(right) }
            return false
        }
        return left == right
    }

    private val UUID = Regex("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")

    private fun dateTime(value: String): Boolean {
        val match = Regex("""^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-]\d{2}:\d{2})$""").matchEntire(value)
            ?: return false
        return runCatching { java.time.Instant.parse(value); true }.getOrDefault(false) && match.groupValues[1] != "0001"
    }
}
