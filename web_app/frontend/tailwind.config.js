/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,jsx}"],
  theme: {
    extend: {
      colors: {
        museum: {
          50:  "#fdf8f0",
          100: "#f9edd9",
          200: "#f2d9ae",
          300: "#e9bf7c",
          400: "#dfa04a",
          500: "#d4842a",
          600: "#b86820",
          700: "#98501d",
          800: "#7c411e",
          900: "#65361c",
        },
      },
    },
  },
  plugins: [],
};
