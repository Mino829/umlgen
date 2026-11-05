package sample.app;

import sample.service.Service;
import sample.domain.*;
import sample.alternate.*;

// Item is deliberately ambiguous; Missing is deliberately unresolved.
public class App {
    Service service;
    Item ambiguous;
    Missing missing;
}
