import { useCallback, useEffect, useState } from "react";

export type EditableMetadata = { title?: string; description?: string; tags?: string[] };
export type ViewerMetadata = {
  version: 1;
  collections: Record<string, EditableMetadata>;
  trajectories: Record<string, EditableMetadata>;
};

export const viewerMetadataStorageKey = "rlviz.viewer-metadata.v1";
const maximumCollections = 64;
const maximumTrajectories = 256;
const maximumTitleLength = 120;
const maximumDescriptionLength = 500;
const maximumTags = 16;
const maximumTagLength = 40;

const emptyMetadata = (): ViewerMetadata => ({ version: 1, collections: {}, trajectories: {} });

function cleanText(value: unknown, maximum: number): string | undefined {
  if (typeof value !== "string") return undefined;
  const cleaned = value.trim().slice(0, maximum);
  return cleaned || undefined;
}

function cleanEntries(value: unknown, maximumEntries: number): Record<string, EditableMetadata> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return Object.fromEntries(Object.entries(value as Record<string, unknown>).slice(-maximumEntries).flatMap(([key, candidate]) => {
    if (!key || key.length > 500 || !candidate || typeof candidate !== "object" || Array.isArray(candidate)) return [];
    const record = candidate as Record<string, unknown>;
    const tags = Array.isArray(record.tags) ? [...new Set(record.tags.flatMap((tag) => {
      const cleaned = cleanText(tag, maximumTagLength)?.toLowerCase();
      return cleaned ? [cleaned] : [];
    }))].slice(0, maximumTags) : [];
    const metadata = { title: cleanText(record.title, maximumTitleLength), description: cleanText(record.description, maximumDescriptionLength), tags: tags.length ? tags : undefined };
    return metadata.title || metadata.description || metadata.tags ? [[key, metadata]] : [];
  }));
}

export function normalizeViewerMetadata(value: unknown): ViewerMetadata {
  if (!value || typeof value !== "object" || Array.isArray(value)) return emptyMetadata();
  const input = value as Record<string, unknown>;
  return {
    version: 1,
    collections: cleanEntries(input.collections, maximumCollections),
    trajectories: cleanEntries(input.trajectories, maximumTrajectories),
  };
}

export function collectionMetadataKey(sourceIds: readonly string[]): string {
  return sourceIds.length ? [...new Set(sourceIds)].sort().join("|").slice(0, 500) : "default";
}

function loadMetadata(): ViewerMetadata {
  try {
    return normalizeViewerMetadata(JSON.parse(localStorage.getItem(viewerMetadataStorageKey) ?? "{}"));
  } catch { return emptyMetadata(); }
}

export function useViewerMetadata() {
  const [metadata, setMetadata] = useState<ViewerMetadata>(loadMetadata);
  useEffect(() => {
    try { localStorage.setItem(viewerMetadataStorageKey, JSON.stringify(metadata)); } catch { /* optional local presentation state */ }
  }, [metadata]);

  const update = useCallback((scope: "collections" | "trajectories", key: string, value: EditableMetadata) => {
    setMetadata((current) => normalizeViewerMetadata({
      ...current,
      [scope]: { ...current[scope], [key]: value },
    }));
  }, []);

  return {
    metadata,
    updateCollection: useCallback((key: string, value: EditableMetadata) => update("collections", key, value), [update]),
    updateTrajectory: useCallback((key: string, value: EditableMetadata) => update("trajectories", key, value), [update]),
  };
}
