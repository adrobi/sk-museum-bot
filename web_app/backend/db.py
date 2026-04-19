import os
from databases import Database
from dotenv import load_dotenv

load_dotenv(dotenv_path=os.path.join(os.path.dirname(__file__), "../../.env"))

DATABASE_URL = os.getenv("DATABASE_URL", "postgresql://user:password@localhost/museum_db")

database = Database(DATABASE_URL)
