import { Parser } from "node-sql-parser";
const parser = new Parser();
const ast = parser.astify("SELECT * FROM dual");
console.log(ast);
