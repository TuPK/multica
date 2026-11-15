export function countOpen(list) {
  return list.filter((item) => item.status === "open").length;
}

export function sumOpen(list) {
  return list.filter((item) => item.status === "open").reduce((total, item) => total + item.amount, 0);
}
