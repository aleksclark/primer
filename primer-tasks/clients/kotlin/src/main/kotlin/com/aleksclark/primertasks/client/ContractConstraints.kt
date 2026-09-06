package com.aleksclark.primertasks.client

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull

/**
 * Interpreter for Go-owned JSON Schema extensions used by Tasks contracts.
 * Unknown `x-*` keys fail closed instead of silently passing.
 */
object ContractConstraints {
    val implemented = setOf("x-maxBytes", "x-nonBlank", "x-equalFields", "x-uniqueNormalized")
    val metadata = setOf("x-manifest", "x-transport")

    fun requireSupported(schema: JsonObject, path: String = "\$") {
        for ((key, value) in schema) {
            if (key.startsWith("x-") && key !in implemented && key !in metadata) {
                error("unsupported contract constraint $key at $path")
            }
            when (key) {
                "x-maxBytes" -> {
                    val limit = value.jsonPrimitive.longOrNull
                    check(schema.stringType() && limit != null && limit > 0) {
                        "x-maxBytes at $path must be a positive integer on a string schema"
                    }
                }
                "x-nonBlank" -> check(schema.stringType() && value.jsonPrimitive.booleanOrNull == true) {
                    "x-nonBlank at $path must be true on a string schema"
                }
                "x-equalFields" -> {
                    val fields = value.jsonArray.map { it.jsonPrimitive.content }
                    check(schema.objectType() && fields.size == 2 && fields.all { it.isNotBlank() }) {
                        "x-equalFields at $path must name exactly two object fields"
                    }
                }
                "x-uniqueNormalized" -> check(schema.arrayType() && value.jsonPrimitive.booleanOrNull == true) {
                    "x-uniqueNormalized at $path must be true on an array schema"
                }
            }
            when (value) {
                is JsonObject -> requireSupported(value, "$path.$key")
                is JsonArray -> value.forEachIndexed { index, child ->
                    if (child is JsonObject) requireSupported(child, "$path.$key[$index]")
                }
                else -> Unit
            }
        }
    }

    fun matches(schema: JsonObject, value: JsonElement): Boolean {
        return try {
            requireSupported(schema)
            matchesUnchecked(schema, value)
        } catch (_: IllegalStateException) {
            false
        }
    }

    private fun matchesUnchecked(schema: JsonObject, value: JsonElement): Boolean {
        schema["allOf"]?.jsonArray?.forEach { part ->
            if (!matchesUnchecked(part.jsonObject, value)) return false
        }
        schema["oneOf"]?.jsonArray?.let { alternatives ->
            if (alternatives.count { matchesUnchecked(it.jsonObject, value) } != 1) return false
        }
        schema["not"]?.let { if (matchesUnchecked(it.jsonObject, value)) return false }
        schema["const"]?.let { if (!jsonEqual(it, value)) return false }
        schema["enum"]?.jsonArray?.let { values ->
            if (values.none { jsonEqual(it, value) }) return false
        }
        schema["if"]?.let { condition ->
            if (matchesUnchecked(condition.jsonObject, value)) {
                val thenSchema = schema["then"]?.jsonObject ?: return false
                if (!matchesUnchecked(thenSchema, value)) return false
            }
        }
        when (schema["type"]?.jsonPrimitive?.contentOrNull) {
            "object" -> if (value !is JsonObject) return false
            "boolean" -> if (value !is JsonPrimitive || value.booleanOrNull == null) return false
            "integer" -> {
                if (value !is JsonPrimitive) return false
                val number = value.doubleOrNull ?: return false
                if (number != kotlin.math.truncate(number) || number > 9007199254740991.0) return false
                schema["minimum"]?.jsonPrimitive?.doubleOrNull?.let { if (number < it) return false }
                schema["maximum"]?.jsonPrimitive?.doubleOrNull?.let { if (number > it) return false }
            }
            "string" -> {
                if (value !is JsonPrimitive) return false
                val text = value.contentOrNull ?: return false
                val runes = text.codePointCount(0, text.length)
                schema["minLength"]?.jsonPrimitive?.longOrNull?.let { if (runes < it) return false }
                schema["maxLength"]?.jsonPrimitive?.longOrNull?.let { if (runes > it) return false }
                schema["x-maxBytes"]?.jsonPrimitive?.longOrNull?.let {
                    if (text.toByteArray(Charsets.UTF_8).size > it) return false
                }
                if (schema["x-nonBlank"]?.jsonPrimitive?.booleanOrNull == true && text.trim().isEmpty()) return false
            }
            "array" -> {
                if (value !is JsonArray) return false
                schema["minItems"]?.jsonPrimitive?.longOrNull?.let { if (value.size < it) return false }
                schema["maxItems"]?.jsonPrimitive?.longOrNull?.let { if (value.size > it) return false }
                schema["items"]?.jsonObject?.let { itemSchema ->
                    if (value.any { !matchesUnchecked(itemSchema, it) }) return false
                }
                if (schema["uniqueItems"]?.jsonPrimitive?.booleanOrNull == true && value.map { it.toString() }.toSet().size != value.size) {
                    return false
                }
                if (schema["x-uniqueNormalized"]?.jsonPrimitive?.booleanOrNull == true) {
                    val normalized = value.map { it.jsonPrimitive.contentOrNull?.trim()?.lowercase() ?: return false }
                    if (normalized.toSet().size != normalized.size) return false
                }
            }
        }
        if (value is JsonObject) {
            schema["required"]?.jsonArray?.forEach { key ->
                if (!value.containsKey(key.jsonPrimitive.content)) return false
            }
            val properties = schema["properties"]?.jsonObject
            for ((key, child) in value) {
                val property = properties?.get(key)?.jsonObject
                if (property == null) {
                    if (schema["additionalProperties"] is JsonPrimitive &&
                        schema["additionalProperties"]?.jsonPrimitive?.booleanOrNull == false
                    ) {
                        return false
                    }
                } else if (!matchesUnchecked(property, child)) {
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

    private fun JsonObject.stringType() = this["type"]?.jsonPrimitive?.contentOrNull == "string"
    private fun JsonObject.objectType() = this["type"]?.jsonPrimitive?.contentOrNull == "object" || this.containsKey("properties")
    private fun JsonObject.arrayType() = this["type"]?.jsonPrimitive?.contentOrNull == "array"

    private fun jsonEqual(left: JsonElement?, right: JsonElement?): Boolean {
        if (left == null || right == null) return left == null && right == null
        if (left is JsonNull && right is JsonNull) return true
        if (left is JsonPrimitive && right is JsonPrimitive) {
            left.booleanOrNull?.let { return it == right.booleanOrNull }
            left.doubleOrNull?.let { return it == right.doubleOrNull }
            return left.content == right.contentOrNull
        }
        return left == right
    }
}
