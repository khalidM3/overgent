// The fixture models the frontend consumer of backend.Refresh(userID).
export function refreshSession(userID: string): string {
  return `backend.Refresh(${userID})`;
}
