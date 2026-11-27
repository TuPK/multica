"use client";

import type { Intent } from "@multica/core/durable/intents";
import { useT } from "../../i18n";

/** The content a person reads before deciding on an intent: title, recipients, body and field changes. */
export function IntentPreview({ intent }: { intent: Intent }) {
  const { t } = useT("durable");
  const preview = intent.preview;
  const title = preview.title || intent.target;

  return (
    <>
      <h3 id={`intent-${intent.id}-title`} className="text-title-sm font-medium break-words">
        {title}
      </h3>

      {preview.recipients.length > 0 ? (
        <p className="text-caption text-muted-foreground">
          <span>{t(($) => $.intent.recipients)}: </span>
          <span className="break-all text-foreground">{preview.recipients.join(", ")}</span>
        </p>
      ) : null}

      {preview.body_markdown ? (
        <p className="whitespace-pre-wrap break-words text-body text-foreground">{preview.body_markdown}</p>
      ) : null}

      {preview.fields.length > 0 ? (
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-caption">
          {preview.fields.map((field, index) => (
            <div key={`${field.label}-${index}`} className="contents">
              <dt className="text-muted-foreground">{field.label}</dt>
              <dd className="flex flex-wrap items-center gap-x-1.5 break-all text-foreground">
                {field.before ? (
                  <>
                    <span className="text-muted-foreground line-through">{field.before}</span>
                    <span aria-hidden className="text-muted-foreground">→</span>
                  </>
                ) : null}
                <span>{field.after}</span>
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
    </>
  );
}
