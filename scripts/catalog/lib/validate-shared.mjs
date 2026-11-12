export const isObject = (value) => value !== null && typeof value === "object" && !Array.isArray(value);
export const isNonEmptyString = (value) => typeof value === "string" && value.trim() !== "";
