/** The end date to send: only a day after the start makes an activity span several days. null clears it. */
export function endDateToSend(date: string, endDate?: string): string | null {
  return endDate && date && endDate > date ? new Date(endDate).toISOString() : null;
}

export function endsBeforeStart(date: string, endDate?: string): boolean {
  return !!endDate && !!date && endDate < date;
}

/** YYYY-MM-DD of an API timestamp, or '' */
export function dayOf(timestamp?: string | null): string {
  return timestamp ? new Date(timestamp).toISOString().slice(0, 10) : '';
}
