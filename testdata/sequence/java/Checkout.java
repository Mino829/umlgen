package sample;

public final class Checkout {
    private static int Input() { return 1; }
    private static boolean Check() { return true; }

    public static void PlaceOrder(boolean valid, Repository repository, Runnable notify) {
        int amount = Pricing.Total(Input());
        LocalRepository local = new LocalRepository();
        local.Save(amount);
        if (valid && Check()) {
            repository.Save(amount);
        } else {
            Store.Reject();
        }
        for (int n = 0; n < 2; n++) {
            Store.Audit(n);
        }
        notify.run();
    }

    public static void Exceptional() {
        try {
            Store.Audit(1);
        } catch (RuntimeException failure) {
            Store.Reject();
        } finally {
            Store.Audit(2);
        }
        Store.Stored();
    }

    public static void Recur() { Recur(); }

    public static void Lambdas() {
        Runnable saved = () -> Store.Stored();
        saved.run();
    }

    public static void Overloaded(int value) { Store.Audit(value); }
    public static void Overloaded(String value) { Store.Reject(); }
}
