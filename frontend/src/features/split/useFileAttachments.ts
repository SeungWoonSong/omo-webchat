import { useEffect, useRef } from "react";
import { useT } from "../../i18n";
import { uploadFiles } from "../terminal/terminal";
import type { ChatSessionRef } from "../workspace/workspace";

import { useSessionDraft } from "./sessionDraft";
import type { FileAttachment } from "./sessionDraft";

let nextAttachmentId = 0;
export function useFileAttachments(session: Pick<ChatSessionRef, "wsId" | "id"> | undefined) {
  const { t } = useT();
  const { attachments: items, setAttachments: setItems } = useSessionDraft(session);
  const generation = useRef(0);
  const controllers = useRef(new Map<number, AbortController>());
  useEffect(() => {
    return () => {
      generation.current++;
      controllers.current.forEach(c => c.abort());
      controllers.current.clear();
      // Keep completed uploads and make interrupted uploads explicitly retryable.
      setItems(items => items.some(item => !item.path && !item.error)
        ? items.map(item => item.path || item.error ? item : { ...item, error: t("chat.uploadNetworkError") })
        : items);
    };
  }, [setItems, t]);
  const update = (id: number, change: Partial<FileAttachment>, epoch: number): void => {
    if (epoch === generation.current) setItems(items => items.map(item => item.id === id ? { ...item, ...change } : item));
  };
  const start = (id: number, file: File): void => {
    const epoch = generation.current;
    if (!session) { update(id, { error: t("chat.uploadUnavailable") }, epoch); return; }
    // Leave room for the multipart envelope under the server's request cap.
    if (file.size > 100 * 1024 * 1024 - 4096) { update(id, { error: t("chat.uploadLimit") }, epoch); return; }
    const controller = new AbortController(); controllers.current.set(id, controller);
    void uploadFiles(session.wsId, session.id, [file], { dir: "uploads", signal: controller.signal, onProgress: progress => update(id, { progress }, epoch) })
      .then(paths => {
        const path = paths[0];
        if (!path) throw new Error(t("toast.uploadFailed"));
        update(id, { path, progress: 100 }, epoch);
      }).catch((err: unknown) => { if (!controller.signal.aborted) update(id, { error: err instanceof TypeError ? t("chat.uploadNetworkError") : err instanceof Error ? err.message : t("toast.uploadFailed") }, epoch); })
      .finally(() => { if (controllers.current.get(id) === controller) controllers.current.delete(id); });
  };
  return {
    items,
    blocked: items.some(item => !item.path),
    paths: items.flatMap(item => item.path ? [item.path] : []),
    add: (files: readonly File[]): void => {
      const added = files.map(file => ({ id: nextAttachmentId++, file, progress: 0 }));
      setItems(items => [...items, ...added]);
      added.forEach(item => start(item.id, item.file));
    },
    remove: (id: number): void => { controllers.current.get(id)?.abort(); controllers.current.delete(id); setItems(items => items.filter(item => item.id !== id)); },
    retry: (id: number): void => {
      const item = items.find(item => item.id === id); if (!item?.error) return;
      setItems(items => items.map(item => item.id === id ? { id, file: item.file, progress: 0 } : item));
      start(id, item.file);
    },
    clear: (): void => { generation.current++; controllers.current.forEach(c => c.abort()); controllers.current.clear(); setItems(() => []); },
  };
}
