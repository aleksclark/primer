package com.aleksclark.primertasks.client

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ContractConstraintsTest {
    private val json = Json { ignoreUnknownKeys = false }

    @Test
    fun maxBytesCountsUtf8NotRunes() {
        val schema = obj("""{"type":"string","minLength":1,"x-maxBytes":12,"x-nonBlank":true}""")
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("\"hello world!\"")))
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("\"${"é".repeat(7)}\"")))
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("\"${"é".repeat(6)}\"")))
    }

    @Test
    fun nonBlankRejectsWhitespace() {
        val schema = obj("""{"type":"string","minLength":1,"x-nonBlank":true}""")
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("\"   \"")))
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("\"source\"")))
    }

    @Test
    fun equalFieldsRequireIdenticalValues() {
        val schema = obj("""{"type":"object","properties":{"sequence":{"type":"integer"},"cursor":{"type":"integer"}},"x-equalFields":["sequence","cursor"]}""")
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("""{"sequence":4,"cursor":4}""")))
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("""{"sequence":4,"cursor":5}""")))
    }

    @Test
    fun uniqueNormalizedIsCaseAndTrimInsensitive() {
        val schema = obj("""{"type":"array","items":{"type":"string","minLength":1,"x-nonBlank":true},"x-uniqueNormalized":true}""")
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("""["Fact","Source"]""")))
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("""["Fact"," fact "]""")))
    }

    @Test
    fun unknownExtensionFailsClosedInsteadOfPassing() {
        val schema = obj("""{"type":"string","x-unknownConstraint":true}""")
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            ContractConstraints.requireSupported(schema)
        }
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("\"ok\"")))
    }

    @Test
    fun jsonTypesAreNotCoerced() {
        val stringSchema = obj("""{"type":"string","minLength":1}""")
        assertFalse(ContractConstraints.matches(stringSchema, json.parseToJsonElement("1")))
        assertFalse(ContractConstraints.matches(stringSchema, json.parseToJsonElement("true")))
        val integerSchema = obj("""{"type":"integer"}""")
        assertFalse(ContractConstraints.matches(integerSchema, json.parseToJsonElement("\"1\"")))
        val booleanSchema = obj("""{"type":"boolean"}""")
        assertFalse(ContractConstraints.matches(booleanSchema, json.parseToJsonElement("\"true\"")))
        val equal = obj("""{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"x-equalFields":["a","b"]}""")
        assertFalse(ContractConstraints.matches(equal, json.parseToJsonElement("""{"a":1,"b":"1"}""")))
    }

    @Test
    fun unknownStandardConstructsFailClosed() {
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            ContractConstraints.requireSupported(obj("""{"type":"string","exclusiveMinimum":1}"""))
        }
        org.junit.Assert.assertThrows(IllegalStateException::class.java) {
            ContractConstraints.requireSupported(obj("""{"anyOf":[{"type":"string"}]}"""))
        }
        assertFalse(ContractConstraints.matches(obj("""{"type":"string","format":"email"}"""), json.parseToJsonElement("\"a@b.c\"")))
    }

    @Test
    fun restApprovedAppConstraintsAreEnforced() {
        val schema = obj("""{"type":"object","properties":{"packageName":{"type":"string","minLength":3,"maxLength":255,"pattern":"^[a-zA-Z][a-zA-Z0-9_]*(\\.[a-zA-Z][a-zA-Z0-9_]*)+$"},"label":{"type":"string","maxLength":80}},"required":["packageName"]}""")
        assertTrue(ContractConstraints.matches(schema, json.parseToJsonElement("""{"packageName":"com.primer.app","label":"App"}""")))
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("""{"packageName":"ab"}""")))
        assertFalse(ContractConstraints.matches(schema, json.parseToJsonElement("""{"packageName":1}""")))
    }

    @Test
    fun frozenDialogueConfigSourceTextAndRubric() {
        val sourceText = obj("""{"type":"string","minLength":1,"x-maxBytes":12000,"x-nonBlank":true}""")
        assertTrue(ContractConstraints.matches(sourceText, json.parseToJsonElement("\"A bounded parent source.\"")))
        assertFalse(ContractConstraints.matches(sourceText, json.parseToJsonElement("\"${"é".repeat(6001)}\"")))
        val rubric = obj("""{"type":"array","minItems":1,"maxItems":20,"uniqueItems":true,"x-uniqueNormalized":true,"items":{"type":"string","minLength":1,"maxLength":500,"x-nonBlank":true}}""")
        assertTrue(ContractConstraints.matches(rubric, json.parseToJsonElement("""["source detail"]""")))
        assertFalse(ContractConstraints.matches(rubric, json.parseToJsonElement("""["Fact"," fact "]""")))
    }

    private fun obj(raw: String): JsonObject = json.parseToJsonElement(raw).jsonObject
}
