import { describe, it, expect } from '@jest/globals';
import { isSafeQuery } from "../src/server";

describe("isSafeQuery Unit Tests", () => {
    describe("Modo: readonly", () => {
        it("deve permitir comandos SELECT puros", () => {
            const result = isSafeQuery("SELECT * FROM usuarios", "readonly");
            expect(result.isSafe).toBe(true);
        });

        it("deve bloquear comandos de INSERT", () => {
            const result = isSafeQuery("INSERT INTO usuarios (nome) VALUES ('Teste')", "readonly");
            expect(result.isSafe).toBe(false);
            expect(result.errorMsg).toContain("Apenas consultas de leitura");
        });

        it("deve bloquear comandos de CREATE", () => {
            const result = isSafeQuery("CREATE TABLE testetmp (id INT)", "readonly");
            expect(result.isSafe).toBe(false);
            expect(result.errorMsg).toContain("Apenas consultas de leitura");
        });

        it("deve bloquear operações destrutivas (DROP)", () => {
            const result = isSafeQuery("DROP TABLE usuarios", "readonly");
            expect(result.isSafe).toBe(false);
            expect(result.errorMsg).toContain("Operações destrutivas");
        });
    });

    describe("Modo: normal", () => {
        it("deve permitir comandos SELECT puros", () => {
            const result = isSafeQuery("SELECT * FROM usuarios", "normal");
            expect(result.isSafe).toBe(true);
        });

        it("deve permitir comandos de INSERT", () => {
            const result = isSafeQuery("INSERT INTO usuarios (nome) VALUES ('Teste')", "normal");
            expect(result.isSafe).toBe(true);
        });

        it("deve permitir comandos de CREATE", () => {
            const result = isSafeQuery("CREATE TABLE testetmp (id INT)", "normal");
            expect(result.isSafe).toBe(true);
        });

        it("deve bloquear operações destrutivas (DROP)", () => {
            const result = isSafeQuery("DROP TABLE usuarios", "normal");
            expect(result.isSafe).toBe(false);
            expect(result.errorMsg).toContain("Operações destrutivas");
        });

        it("deve bloquear operações destrutivas (TRUNCATE)", () => {
            const result = isSafeQuery("TRUNCATE TABLE usuarios", "normal");
            expect(result.isSafe).toBe(false);
            expect(result.errorMsg).toContain("Operações destrutivas");
        });
    });

    describe("Modo: teste", () => {
        it("deve permitir comandos SELECT puros", () => {
            const result = isSafeQuery("SELECT * FROM usuarios", "teste");
            expect(result.isSafe).toBe(true);
        });

        it("deve permitir comandos de INSERT", () => {
            const result = isSafeQuery("INSERT INTO usuarios (nome) VALUES ('Teste')", "teste");
            expect(result.isSafe).toBe(true);
        });

        it("deve permitir comandos de CREATE", () => {
            const result = isSafeQuery("CREATE TABLE testetmp (id INT)", "teste");
            expect(result.isSafe).toBe(true);
        });

        it("deve permitir operações destrutivas (DROP)", () => {
            const result = isSafeQuery("DROP TABLE usuarios", "teste");
            expect(result.isSafe).toBe(true);
        });

        it("deve permitir operações destrutivas (TRUNCATE)", () => {
            const result = isSafeQuery("TRUNCATE TABLE usuarios", "teste");
            expect(result.isSafe).toBe(true);
        });
    });
});
