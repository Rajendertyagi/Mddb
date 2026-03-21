export function sanitizeInput(input: string, maxLength: number): string {
  return input
    .replace(/<[^>]*>/g, '') // strip HTML tags
    .trim()
    .slice(0, maxLength);
}
