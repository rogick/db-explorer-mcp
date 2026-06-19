import { describe, it, expect } from '@jest/globals';
import { isSafeQuery } from "../src/server";

describe("isSafeQuery Security Protections", () => {
    describe("Bypass attempts with comments", () => {
        const queries = [
            "DROP/**/TABLE users",
            "DROP /* bypass */ TABLE users",
            "DROP\n-- bypass\nTABLE users",
            "SELECT * FROM users; /* */ DROP TABLE users",
            "TRUNCATE/**/TABLE users",
            "DELETE/**/FROM users",
            "INSERT/**/INTO users (id) VALUES (1)"
        ];

        it.each(queries)("should block comment bypass in readonly mode: %s", (query: string) => {
            const result = isSafeQuery(query, "readonly");
            expect(result.isSafe).toBe(false);
            expect(result.errorMsg).toBeDefined();
        });

        it.each(queries)("should block comment bypass in normal mode (if applicable): %s", (query: string) => {
            const result = isSafeQuery(query, "normal");
            // INSERT might be allowed in normal mode, but DROP, TRUNCATE, ALTER are forbidden.
            if (query.toUpperCase().includes("DROP") || query.toUpperCase().includes("TRUNCATE")) {
                expect(result.isSafe).toBe(false);
                expect(result.errorMsg).toBeDefined();
            }
        });
    });

    describe("Bypass attempts with newlines and multiple spaces", () => {
        const queries = [
            "DROP\nTABLE users",
            "DROP\tTABLE users",
            "DROP\r\nTABLE users",
            "TRUNCATE    TABLE users",
            "DELETE\nFROM users",
            "UPDATE\r\nusers SET id=1"
        ];

        it.each(queries)("should block newline/whitespace bypass in readonly mode: %s", (query: string) => {
            const result = isSafeQuery(query, "readonly");
            expect(result.isSafe).toBe(false);
            expect(result.errorMsg).toBeDefined();
        });

        it.each(queries)("should block newline/whitespace bypass in normal mode (for DDL/DCL): %s", (query: string) => {
            const result = isSafeQuery(query, "normal");
            if (query.toUpperCase().includes("DROP") || query.toUpperCase().includes("TRUNCATE")) {
                expect(result.isSafe).toBe(false);
                expect(result.errorMsg).toBeDefined();
            }
        });
    });

    describe("Bypass attempts with multiple statements", () => {
        const queries = [
            "SELECT * FROM users; DROP TABLE users;",
            "SELECT 1; TRUNCATE TABLE users;",
            "SELECT 1; DELETE FROM users;",
            "SELECT * FROM users \n ; DROP TABLE users",
            "SELECT 1; UPDATE users SET name = 'hacked';"
        ];

        it.each(queries)("should block multiple statements bypass in readonly mode: %s", (query: string) => {
            const result = isSafeQuery(query, "readonly");
            expect(result.isSafe).toBe(false);
            expect(result.errorMsg).toBeDefined();
        });

        it.each(queries)("should block multiple statements bypass in normal mode (if containing forbidden statements): %s", (query: string) => {
            const result = isSafeQuery(query, "normal");
            if (query.toUpperCase().includes("DROP") || query.toUpperCase().includes("TRUNCATE")) {
                expect(result.isSafe).toBe(false);
                expect(result.errorMsg).toBeDefined();
            }
        });
    });

    describe("Case insensitivity checks", () => {
        const queries = [
            "drop table users",
            "DrOp TaBlE users",
            "tRuNcAtE table users",
            "dElEtE from users",
            "AlTeR tAbLe users"
        ];

        it.each(queries)("should block mixed case in readonly mode: %s", (query: string) => {
            const result = isSafeQuery(query, "readonly");
            expect(result.isSafe).toBe(false);
            expect(result.errorMsg).toBeDefined();
        });

        it.each(queries)("should block mixed case in normal mode (for DDL/DCL): %s", (query: string) => {
            const result = isSafeQuery(query, "normal");
            if (query.toUpperCase().includes("DROP") || query.toUpperCase().includes("TRUNCATE") || query.toUpperCase().includes("ALTER")) {
                expect(result.isSafe).toBe(false);
                expect(result.errorMsg).toBeDefined();
            }
        });
    });
});
