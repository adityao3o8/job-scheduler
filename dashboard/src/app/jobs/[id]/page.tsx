"use client";

import { useParams, useRouter } from "next/navigation";
import { useEffect } from "react";

/** Deep-link support: /jobs/:id opens the jobs explorer with the detail drawer. */
export default function JobDetailRedirect() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  useEffect(() => {
    if (id) router.replace(`/jobs?job=${id}`);
  }, [id, router]);

  return null;
}
