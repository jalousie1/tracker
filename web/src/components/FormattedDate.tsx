"use client";
import { useEffect, useState } from "react";

interface FormattedDateProps {
    date: string | Date;
    includeTime?: boolean;
}

export function FormattedDate({ date, includeTime = true }: FormattedDateProps) {
    const [mounted, setMounted] = useState(false);

    useEffect(() => {
        setMounted(true);
    }, []);

    if (!mounted) {
        return <span className="animate-pulse bg-neutral-800 rounded px-2">...</span>;
    }

    if (!date) {
        return <span className="text-neutral-500 italic text-[10px]">N/A</span>;
    }

    const d = typeof date === "string" ? new Date(date) : date;

    if (!d || isNaN(d.getTime())) {
        return <span className="text-neutral-500 italic text-[10px]">Data inválida</span>;
    }

    return (
        <span>
            {includeTime
                ? d.toLocaleString("pt-BR")
                : d.toLocaleDateString("pt-BR")}
        </span>
    );
}
