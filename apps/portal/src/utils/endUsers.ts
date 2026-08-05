export const END_USER_LOOKUP_PAGE_SIZE = 100;

interface EndUserPage<T extends { userId: string }> {
  items?: T[];
  total?: number;
}

export async function findEndUserById<T extends { userId: string }>(
  fetchPage: (params: { page: number; size: number }) => Promise<EndUserPage<T>>,
  userId: string
): Promise<T | null> {
  let page = 1;

  for (;;) {
    const response = await fetchPage({ page, size: END_USER_LOOKUP_PAGE_SIZE });
    const items = response.items ?? [];
    const matched = items.find((item) => item.userId === userId);
    if (matched) return matched;
    if (!items.length || page * END_USER_LOOKUP_PAGE_SIZE >= (response.total ?? 0)) return null;
    page += 1;
  }
}
