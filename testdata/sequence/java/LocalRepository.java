package sample;

public final class LocalRepository implements Repository {
    public LocalRepository() { Store.Audit(0); }
    public void Save(int amount) { Store.Stored(); }
}
