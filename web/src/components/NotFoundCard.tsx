"use client";

import { motion } from "framer-motion";
import { ArrowLeft, SearchX } from "lucide-react";
import Link from "next/link";

export function NotFoundCard() {
  return (
    <div className="flex flex-col items-center justify-center p-6 w-full max-w-md mx-auto">
      <motion.div
        initial={{ opacity: 0, scale: 0.9 }}
        animate={{ opacity: 1, scale: 1 }}
        className="glass p-8 rounded-3xl border border-neutral-800 bg-neutral-900/50 text-center w-full shadow-2xl"
      >
        <div className="w-16 h-16 rounded-full bg-neutral-800/50 flex items-center justify-center mx-auto mb-6 text-neutral-400">
            <SearchX className="w-8 h-8" />
        </div>
        
        <h2 className="text-xl font-semibold text-white mb-2">usuario nao encontrado</h2>
        <p className="text-neutral-400 text-sm mb-8">
            nao existe perfil para esse id no momento.
        </p>

        <Link 
            href="/"
            className="inline-flex items-center gap-2 px-6 py-3 rounded-full bg-white text-black font-medium text-sm hover:bg-neutral-200 transition-colors"
        >
            <ArrowLeft className="w-4 h-4" />
            voltar
        </Link>
      </motion.div>
    </div>
  );
}

