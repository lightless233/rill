import { useEffect, useMemo, useRef, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { AlertCircle, Clock, Loader2, Lock, RotateCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  APIError,
  shareAPI,
  type ShareAccessResponse,
  type ShareContentResponse,
} from "@/lib/api";
import { processArticleContent } from "@/lib/content";
import { useI18n } from "@/lib/i18n";
import { toSafeExternalUrl } from "@/lib/safe-url";
import { formatDate } from "@/lib/utils";

export const Route = createLazyFileRoute("/s/$token")({
  component: PublicSharePage,
});

function isShareContent(
  value: ShareAccessResponse | undefined,
): value is ShareContentResponse {
  return value !== undefined && "content" in value;
}

function PublicSharePage() {
  const { token } = Route.useParams();
  const { t, locale } = useI18n();
  const [unlocked, setUnlocked] = useState<ShareContentResponse | null>(null);
  const [terminalState, setTerminalState] = useState<
    "expired" | "notfound" | null
  >(null);
  const [expired, setExpired] = useState(false);
  const [passcode, setPasscode] = useState("");
  const [passcodeError, setPasscodeError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const passcodeInputRef = useRef<HTMLInputElement | null>(null);

  const shareQuery = useQuery({
    queryKey: ["shares", token],
    queryFn: async () => (await shareAPI.get(token)).data!,
    retry: false,
    staleTime: 0,
  });
  const { data, error, isPending, refetch } = shareQuery;
  const errorStatus = error instanceof APIError ? error.status : null;
  const expiresAt = data?.expires_at ?? unlocked?.expires_at;

  // The server is the authority, but a page already open must not keep showing
  // the snapshot after its expiry. setTimeout is capped at ~24.8 days, so
  // long-lived shares reschedule until the real expiry passes.
  useEffect(() => {
    if (!expiresAt || expired) return;

    let timer: number;
    const schedule = () => {
      const remainingMs = expiresAt * 1000 - Date.now();
      if (remainingMs <= 0) {
        setExpired(true);
        setUnlocked(null);
        return;
      }
      timer = window.setTimeout(schedule, Math.min(remainingMs, 2_147_483_647));
    };
    schedule();

    return () => window.clearTimeout(timer);
  }, [expiresAt, expired]);

  // Recheck on tab return and bfcache restore. A stale local clock is corrected
  // by the server's authoritative 410/404 response.
  useEffect(() => {
    if (!expiresAt) return;

    const recheck = () => {
      if (Date.now() >= expiresAt * 1000) {
        setExpired(true);
        setUnlocked(null);
        return;
      }
      void refetch();
    };
    const handleVisibility = () => {
      if (document.visibilityState === "visible") {
        recheck();
      }
    };
    const handlePageShow = () => recheck();

    document.addEventListener("visibilitychange", handleVisibility);
    window.addEventListener("pageshow", handlePageShow);
    return () => {
      document.removeEventListener("visibilitychange", handleVisibility);
      window.removeEventListener("pageshow", handlePageShow);
    };
  }, [expiresAt, refetch]);

  const showExpired =
    expired || terminalState === "expired" || errorStatus === 410;
  const showNotFound = terminalState === "notfound" || errorStatus === 404;
  const content = showExpired || showNotFound
    ? null
    : isShareContent(data)
      ? data
      : unlocked;
  const requiresPasscode =
    data !== undefined && !isShareContent(data) && "requires_passcode" in data;

  const renderedHtml = useMemo(
    () =>
      content ? processArticleContent(content.content, content.link) : "",
    [content],
  );
  const safeLink = content ? toSafeExternalUrl(content.link) : null;
  const linkHost = safeLink ? new URL(safeLink).hostname : "";
  const expiryLabel = expiresAt
    ? new Date(expiresAt * 1000).toLocaleString(locale)
    : "";

  const handleAccess = async (event: React.FormEvent) => {
    event.preventDefault();
    if (passcode.length !== 4 || submitting) return;

    setSubmitting(true);
    setPasscodeError(null);
    try {
      const response = await shareAPI.access(token, passcode);
      setUnlocked(response.data!);
      setPasscode("");
    } catch (accessError) {
      if (accessError instanceof APIError) {
        if (accessError.status === 403) {
          setPasscodeError(t("share.public.passcode.invalid"));
          setPasscode("");
          passcodeInputRef.current?.focus();
        } else if (accessError.status === 429) {
          const minutes = Math.max(
            1,
            Math.ceil((accessError.retryAfter ?? 60) / 60),
          );
          setPasscodeError(
            t("share.public.passcode.rateLimited", { minutes }),
          );
        } else if (accessError.status === 410) {
          setTerminalState("expired");
        } else if (accessError.status === 404) {
          setTerminalState("notfound");
        } else {
          setPasscodeError(t("share.public.error.description"));
        }
      } else {
        setPasscodeError(t("share.public.error.description"));
      }
    } finally {
      setSubmitting(false);
    }
  };

  let body: React.ReactNode;
  if (showExpired) {
    body = (
      <ShareState
        icon={<Clock className="h-6 w-6" />}
        title={t("share.public.expired.title")}
        description={t("share.public.expired.description")}
      />
    );
  } else if (showNotFound) {
    body = (
      <ShareState
        icon={<AlertCircle className="h-6 w-6" />}
        title={t("share.public.notFound.title")}
        description={t("share.public.notFound.description")}
      />
    );
  } else if (content) {
    body = (
      <article className="mx-auto w-full max-w-3xl px-5 py-8 sm:px-6 sm:py-12">
        <h1 className="article-typography text-[28px] font-bold leading-[1.3]">
          {content.title}
        </h1>
        <div className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-1.5 text-sm">
          {content.feed_name && (
            <span className="max-w-60 truncate rounded bg-muted px-2 py-1 text-xs font-medium text-muted-foreground">
              {content.feed_name}
            </span>
          )}
          {content.pub_date > 0 && (
            <span className="text-muted-foreground">
              {formatDate(content.pub_date)}
            </span>
          )}
          {safeLink && (
            <a
              href={safeLink}
              target="_blank"
              rel="noopener noreferrer"
              className="truncate text-primary hover:underline"
            >
              {linkHost}
            </a>
          )}
        </div>
        <div
          className="typeset typeset-article mt-8 min-w-0"
          dangerouslySetInnerHTML={{ __html: renderedHtml }}
        />
        <p className="mt-10 border-t pt-4 text-xs text-muted-foreground">
          {t("share.public.expires", { date: expiryLabel })}
        </p>
      </article>
    );
  } else if (requiresPasscode) {
    body = (
      <div className="mx-auto w-full max-w-sm px-5 py-10">
        <div className="space-y-2 text-center">
          <div className="mx-auto flex h-11 w-11 items-center justify-center rounded-full bg-muted">
            <Lock className="h-5 w-5 text-muted-foreground" />
          </div>
          <h1 className="text-xl font-semibold">
            {t("share.public.passcode.title")}
          </h1>
          <p className="text-sm text-muted-foreground">
            {t("share.public.passcode.description")}
          </p>
        </div>

        <form onSubmit={handleAccess} className="mt-6 space-y-3">
          <Input
            ref={passcodeInputRef}
            value={passcode}
            onChange={(event) =>
              setPasscode(
                event.target.value
                  .replace(/[^a-zA-Z0-9]/g, "")
                  .toUpperCase()
                  .slice(0, 4),
              )
            }
            placeholder={t("share.public.passcode.placeholder")}
            aria-label={t("share.public.passcode.placeholder")}
            autoComplete="off"
            spellCheck={false}
            maxLength={4}
            autoFocus
            className="text-center font-mono uppercase tracking-[0.35em]"
          />
          {passcodeError && (
            <p role="alert" className="text-sm text-destructive">
              {passcodeError}
            </p>
          )}
          <Button
            type="submit"
            className="w-full"
            disabled={submitting || passcode.length !== 4}
          >
            {submitting ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                {t("share.public.passcode.submitting")}
              </>
            ) : (
              t("share.public.passcode.submit")
            )}
          </Button>
        </form>

        <p className="mt-6 text-center text-xs text-muted-foreground">
          {t("share.public.expires", { date: expiryLabel })}
        </p>
      </div>
    );
  } else if (isPending) {
    body = (
      <div className="flex items-center justify-center py-16">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  } else {
    body = (
      <ShareState
        icon={<AlertCircle className="h-6 w-6" />}
        title={t("share.public.error.title")}
        description={t("share.public.error.description")}
        action={
          <Button variant="outline" onClick={() => void refetch()}>
            <RotateCw className="h-4 w-4" />
            {t("common.retry")}
          </Button>
        }
      />
    );
  }

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b">
        <div className="mx-auto flex h-14 max-w-3xl items-center gap-2 px-5 sm:px-6">
          <img
            src="/icon-96.png"
            alt={t("common.appLogo")}
            width={24}
            height={24}
            className="h-6 w-6 rounded-md"
          />
          <span className="text-sm font-semibold">Rill</span>
        </div>
      </header>
      <main className="flex min-h-[calc(100vh-3.5rem)] flex-col justify-center">
        {body}
      </main>
    </div>
  );
}

function ShareState({
  icon,
  title,
  description,
  action,
}: {
  icon: React.ReactNode;
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="mx-auto w-full max-w-sm px-5 py-10 text-center">
      <div className="mx-auto flex h-11 w-11 items-center justify-center rounded-full bg-muted text-muted-foreground">
        {icon}
      </div>
      <h1 className="mt-4 text-xl font-semibold">{title}</h1>
      <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      {action && <div className="mt-5 flex justify-center">{action}</div>}
    </div>
  );
}
