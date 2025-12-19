import { SearchInput } from "@/components/SearchInput";

export default function Home() {
  return (
    <div className="flex flex-col items-center justify-center w-full max-w-2xl px-4 space-y-12">
      <div className="text-center space-y-4">
        <h1 className="text-4xl sm:text-6xl font-bold tracking-tighter text-transparent bg-clip-text bg-gradient-to-b from-white to-white/40">
          Identity Archive
        </h1>
        <p className="text-neutral-400 text-sm sm:text-base max-w-md mx-auto">
          pesquise e explore historico e conexoes de identidades.
        </p>
      </div>

      <div className="w-full flex justify-center">
        <SearchInput />
      </div>
      
      <div className="absolute bottom-8 text-neutral-600 text-xs">
         &copy; {new Date().getFullYear()} Identity Archive
      </div>
    </div>
  );
}
