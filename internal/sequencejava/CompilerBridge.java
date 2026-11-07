import java.io.*;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.nio.file.Path;
import java.security.MessageDigest;
import java.util.*;
import javax.lang.model.element.*;
import javax.lang.model.type.*;
import javax.lang.model.util.*;
import javax.tools.*;
import com.sun.source.tree.*;
import com.sun.source.util.*;

/** Trusted compiler-only bridge. No target class loading, generation, or execution. */
class CompilerBridge {
    static Map<String,Object> obj(Object... pairs) {
        Map<String,Object> result = new LinkedHashMap<>();
        for (int i=0; i<pairs.length; i+=2) result.put((String)pairs[i], pairs[i+1]);
        return result;
    }
    static class Input extends SimpleJavaFileObject {
        final String path, root, text;
        final int[] bytes, lines, columns;
        Input(String path, String root, String text) {
            super(Path.of(path).toUri(), JavaFileObject.Kind.SOURCE);
            this.path=path; this.root=root; this.text=text;
            bytes=new int[text.length()+1]; lines=new int[bytes.length]; columns=new int[bytes.length];
            int offset=0, line=1, column=1;
            for (int i=0; i<text.length();) {
                int cp=text.codePointAt(i), count=Character.charCount(cp);
                int width=cp<0x80?1:cp<0x800?2:cp<0x10000?3:4;
                for (int j=0;j<count;j++) { bytes[i+j]=offset; lines[i+j]=line; columns[i+j]=column; }
                offset+=width;
                if (cp=='\n') { line++; column=1; } else column+=width;
                i+=count;
            }
            bytes[text.length()]=offset; lines[text.length()]=line; columns[text.length()]=column;
        }
        @Override public CharSequence getCharContent(boolean ignoreEncodingErrors) { return text; }
        Map<String,Object> position(long at) {
            int i=(int)Math.max(0, Math.min(text.length(), at));
            return obj("Offset",bytes[i],"Line",lines[i],"Column",columns[i]);
        }
        Map<String,Object> span(long start,long end) { return obj("Start",position(start),"End",position(Math.max(start,end))); }
    }
    static String field(DataInputStream in) throws IOException {
        long size=Integer.toUnsignedLong(in.readInt());
        if (size>Integer.MAX_VALUE) throw new IOException("input field too large");
        byte[] bytes=in.readNBytes((int)size);
        if (bytes.length!=size) throw new EOFException("truncated framed input");
        return new String(bytes,StandardCharsets.UTF_8);
    }
    public static void main(String[] args) throws Exception {
        DataInputStream in=new DataInputStream(new BufferedInputStream(System.in));
        byte[] magic=in.readNBytes(8);
        boolean activity=Arrays.equals(magic,"UMLACTJ1".getBytes(StandardCharsets.US_ASCII));
        if (!activity && !Arrays.equals(magic, "UMLSEQJ1".getBytes(StandardCharsets.US_ASCII))) throw new IOException("invalid protocol magic");
        int release=in.readInt(), count=in.readInt();
        long maxNodes=activity?in.readLong():0;
        if(activity && maxNodes<1) throw new IOException("invalid activity node limit");
        if (release<17 || count<0) throw new IOException("invalid release or source count");
        List<Input> inputs=new ArrayList<>();
        for (int i=0;i<count;i++) inputs.add(new Input(field(in),field(in),field(in)));
        if (!activity && in.read()!=-1) throw new IOException("trailing framed input");
        inputs.sort(Comparator.comparing(s->s.path));
        Engine engine=new Engine(inputs);
        JavaCompiler compiler=ToolProvider.getSystemJavaCompiler();
        if (compiler==null) throw new IllegalStateException("installed Java runtime does not include the JDK compiler");
        DiagnosticCollector<JavaFileObject> diagnostics=new DiagnosticCollector<>();
        try (StandardJavaFileManager manager=compiler.getStandardFileManager(diagnostics,Locale.ROOT,StandardCharsets.UTF_8)) {
            Path empty=Path.of(args[0]);
            manager.setLocationFromPaths(StandardLocation.CLASS_PATH,List.of(empty));
            manager.setLocationFromPaths(StandardLocation.SOURCE_PATH,List.of(empty));
            manager.setLocationFromPaths(StandardLocation.ANNOTATION_PROCESSOR_PATH,List.of(empty));
            List<String> options=List.of("--release",Integer.toString(release),"-proc:none","-implicit:none","-classpath",empty.toString(),"-sourcepath",empty.toString(),"-processorpath",empty.toString(),"-encoding","UTF-8","-Xlint:none");
            JavacTask task=(JavacTask)compiler.getTask(new StringWriter(),manager,diagnostics,options,null,inputs);
            engine.trees=Trees.instance(task); engine.elements=task.getElements(); engine.types=task.getTypes();
            try {
                for (CompilationUnitTree unit:task.parse()) engine.units.add(unit);
                engine.captureParseErrors(diagnostics.getDiagnostics());
                task.analyze();
            } catch (Exception failure) { engine.diagnostic("javac.analysis",failure.toString(),null,null,true); }
            for (Diagnostic<? extends JavaFileObject> d:diagnostics.getDiagnostics()) {
                Input source=engine.source(d.getSource());
                engine.diagnostics.add(obj("code",d.getCode(),"message",d.getMessage(Locale.ROOT),"file",source==null?"":source.path,"span",source==null?obj():source.span(d.getStartPosition(),d.getEndPosition()),"error",d.getKind()==Diagnostic.Kind.ERROR));
            }
            engine.extract(!activity);
        }
        Writer stdout=new OutputStreamWriter(System.out,StandardCharsets.UTF_8);
        StringBuilder out=new StringBuilder(); json(engine.program(),out);
        stdout.write(out.toString()); stdout.write('\n'); stdout.flush();
        if(activity) {
            String selected=field(in);
            if(in.read()!=-1) throw new IOException("trailing framed selection");
            Object result=obj();
            for(Decl d:engine.declarations) if(d.id().equals(selected)) {
                result=new Activity(engine,d,maxNodes).build(); break;
            }
            out.setLength(0); json(result,out);
            stdout.write(out.toString()); stdout.flush();
        }
    }
    static class Decl {
        final Map<String,Object> model;
        final Input source;
        final CompilationUnitTree unit;
        final Tree tree, body;
        final ExecutableElement element;
        Decl(Map<String,Object> model,Input source,CompilationUnitTree unit,Tree tree,Tree body,ExecutableElement element) {
            this.model=model; this.source=source; this.unit=unit; this.tree=tree; this.body=body; this.element=element;
        }
        String id() { return (String)model.get("id"); }
    }
    static class Engine {
        final List<Input> inputs;
        final Map<URI,Input> sourcesByURI=new HashMap<>();
        final List<CompilationUnitTree> units=new ArrayList<>();
        final List<Object> diagnostics=new ArrayList<>();
        final List<Decl> declarations=new ArrayList<>();
        final List<TypeElement> selectedTypes=new ArrayList<>();
        final IdentityHashMap<ExecutableElement,Decl> methods=new IdentityHashMap<>();
        final IdentityHashMap<Tree,TreePath> paths=new IdentityHashMap<>();
        final Set<String> ids=new HashSet<>();
        final Map<Input,List<Long>> parseErrors=new IdentityHashMap<>();
        Trees trees; Elements elements; Types types;
        Engine(List<Input> inputs) {
            this.inputs=inputs;
            for(Input source:inputs) sourcesByURI.put(source.toUri(),source);
        }
        Input source(JavaFileObject file) { return file==null?null:sourcesByURI.get(file.toUri()); }
        Input source(CompilationUnitTree unit) { return source(unit.getSourceFile()); }
        long start(CompilationUnitTree unit,Tree tree) { return trees.getSourcePositions().getStartPosition(unit,tree); }
        long end(CompilationUnitTree unit,Tree tree) { return trees.getSourcePositions().getEndPosition(unit,tree); }
        Object span(CompilationUnitTree unit,Tree tree) { return source(unit).span(start(unit,tree),end(unit,tree)); }
        void captureParseErrors(List<Diagnostic<? extends JavaFileObject>> errors) {
            for(Diagnostic<? extends JavaFileObject> error:errors) {
                Input source=source(error.getSource());
                if(error.getKind()==Diagnostic.Kind.ERROR && source!=null) {
                    parseErrors.computeIfAbsent(source,key->new ArrayList<>()).add(error.getPosition());
                }
            }
        }
        boolean damagedByParser(CompilationUnitTree unit,Tree tree) {
            List<Long> positions=parseErrors.get(source(unit));
            if(positions==null) return false;
            long first=start(unit,tree), last=end(unit,tree);
            for(long position:positions) {
                if(position<0 || (position>=first && (last<0 || position<=last))) return true;
            }
            return false;
        }
        void diagnostic(String code,String message,CompilationUnitTree unit,Tree tree,boolean error) {
            diagnostics.add(obj("code",code,"message",message,"file",unit==null?"":source(unit).path,"span",tree==null?obj():span(unit,tree),"error",error));
        }
        Element element(Tree tree) {
            TreePath path=paths.get(tree);
            if(path==null) return null;
            try { return trees.getElement(path); } catch (RuntimeException ignored) { return null; }
        }
        String owner(Element element) {
            if (element instanceof TypeElement type) {
                if(type.getNestingKind()==NestingKind.LOCAL || type.getNestingKind()==NestingKind.ANONYMOUS) return elements.getBinaryName(type).toString();
                String name=type.getQualifiedName().toString();
                if (!name.isEmpty()) return name;
                return elements.getBinaryName(type).toString();
            }
            return element==null?"<unknown>":owner(element.getEnclosingElement());
        }
        String selector(ExecutableElement e) {
            List<String> parameters=new ArrayList<>();
            for(VariableElement p:e.getParameters()) parameters.add(p.asType().toString());
            return owner(e.getEnclosingElement())+"."+e.getSimpleName()+"("+String.join(",",parameters)+")";
        }
        void add(CompilationUnitTree unit,Tree tree,Tree body,ExecutableElement e,String name,String qualified,String selector,String participant,String returns) {
            Input source=source(unit);
            String id=source.root+"::"+selector;
            if (!ids.add(id)) {
                diagnostic("java.duplicate-callable","Duplicate callable identity: "+selector,unit,tree,true);
                id=id+"@"+start(unit,tree);
                if (!ids.add(id)) return;
            }
            Map<String,Object> model=obj("id",id,"selector",selector,"qualified",qualified,"name",name,"participant",source.root+"::"+participant,"returns",returns,"file",source.path,"span",span(unit,tree),"body",new ArrayList<>());
            Decl d=new Decl(model,source,unit,tree,body,e); declarations.add(d);
            if(e!=null) methods.put(e,d);
        }
        void extract(boolean sequenceBodies) {
            for (CompilationUnitTree unit:units) {
                new TreePathScanner<Void,Void>() {
                    String className="<source>";
                    @Override public Void scan(Tree tree,Void unused) {
                        if(tree!=null) {
                            TreePath parent=getCurrentPath();
                            TreePath path=parent==null ? new TreePath((CompilationUnitTree)tree) : new TreePath(parent,tree);
                            paths.put(tree,path);
                        }
                        return super.scan(tree,unused);
                    }
                    @Override public Void visitClass(ClassTree tree,Void unused) {
                        String previous=className;
                        Element e=trees.getElement(getCurrentPath());
                        if(e instanceof TypeElement type) selectedTypes.add(type);
                        className=e==null?previous+".<class>@"+start(unit,tree):owner(e);
                        super.visitClass(tree,unused); className=previous; return null;
                    }
                    @Override public Void visitMethod(MethodTree tree,Void unused) {
                        // Attribution can synthesize a body and <any> signature
                        // while recovering malformed declarations. Such a tree
                        // is not a source-backed entry or nested callable.
                        if(damagedByParser(unit,tree)) return null;
                        if(end(unit,tree)>=0 && tree.getBody()!=null) {
                            Element bound=trees.getElement(getCurrentPath());
                            if(bound instanceof ExecutableElement e) {
                                String name=e.getSimpleName().toString();
                                add(unit,tree,tree.getBody(),e,name,owner(e.getEnclosingElement())+"."+name,selector(e),className,e.getReturnType().toString());
                            } else diagnostic("java.unbound-declaration","Compiler could not bind callable declaration",unit,tree,true);
                        }
                        return super.visitMethod(tree,unused);
                    }
                    @Override public Void visitLambdaExpression(LambdaExpressionTree tree,Void unused) {
                        String qualified=className+".<lambda>@"+start(unit,tree);
                        List<String> params=new ArrayList<>();
                        for(VariableTree p:tree.getParameters()) {
                            Element e=trees.getElement(new TreePath(getCurrentPath(),p));
                            params.add(e==null?"?":e.asType().toString());
                        }
                        add(unit,tree,tree.getBody(),null,"<lambda>",qualified,qualified+"("+String.join(",",params)+")",className,"inferred functional result");
                        return super.visitLambdaExpression(tree,unused);
                    }
                    @Override public Void visitVariable(VariableTree tree,Void unused) {
                        Tree parent=getCurrentPath().getParentPath().getLeaf();
                        if(parent instanceof ClassTree && tree.getInitializer()!=null) {
                            String qualified=className+".<field:"+tree.getName()+">@"+start(unit,tree);
                            add(unit,tree,tree.getInitializer(),null,"<field:"+tree.getName()+">",qualified,qualified+"()",className,tree.getType()==null?"":tree.getType().toString());
                        }
                        return super.visitVariable(tree,unused);
                    }
                    @Override public Void visitBlock(BlockTree tree,Void unused) {
                        TreePath parent=getCurrentPath().getParentPath();
                        if(parent!=null && parent.getLeaf() instanceof ClassTree) {
                            String name=tree.isStatic()?"<clinit>":"<initializer>";
                            String qualified=className+"."+name+"@"+start(unit,tree);
                            add(unit,tree,tree,null,name,qualified,qualified+"()",className,"void");
                        }
                        return super.visitBlock(tree,unused);
                    }
                }.scan(unit,null);
            }
            if(sequenceBodies) for(Decl d:declarations) {
                List<Object> body=new ArrayList<>();
                if(d.body==null) body.add(event(d,"unsupported","Declaration has no selected source body",d.tree));
                else {
                    if(d.element==null || d.element.getKind()==ElementKind.CONSTRUCTOR) body.add(event(d,"note","Initializer/lambda bodies are separate regions; implicit initialization is not simulated",d.tree));
                    statement(d,d.body,body);
                }
                d.model.put("body",body);
            }
            declarations.sort(Comparator.comparing(Decl::id));
        }
        Map<String,Object> event(Decl d,String kind,String label,Tree tree) {
            return obj("kind",kind,"label",label,"file",d.source.path,"span",span(d.unit,tree));
        }
        void unsupported(Decl d,Tree tree,List<Object> out,String reason) {
            diagnostic("java.unsupported",reason,d.unit,tree,false);
            out.add(event(d,"unsupported",reason,tree));
        }
        String text(Decl d,Tree tree) {
            long a=start(d.unit,tree),b=end(d.unit,tree);
            if(a<0||b<a) return tree.getKind().toString();
            String value=d.source.text.substring((int)a,(int)Math.min(b,d.source.text.length())).replaceAll("\\s+"," ");
            return value.length()>180?value.substring(0,180)+"…":value;
        }
        List<Object> region(Decl d,Tree tree) { List<Object> result=new ArrayList<>(); statement(d,tree,result); return result; }
        boolean statement(Decl d,Tree tree,List<Object> out) {
            if(tree==null) return false;
            if(end(d.unit,tree)<0) return false; // javac's synthetic default constructor/super call
            if(tree instanceof BlockTree block) {
                for(StatementTree child:block.getStatements()) if(statement(d,child,out)) return true;
                return false;
            }
            if(tree instanceof ExpressionStatementTree value) { expression(d,value.getExpression(),out); return false; }
            if(tree instanceof VariableTree variable) { expression(d,variable.getInitializer(),out); return false; }
            if(tree instanceof ReturnTree value) { expression(d,value.getExpression(),out); out.add(event(d,"return","return (path ends)",tree)); return true; }
            if(tree instanceof ThrowTree value) { expression(d,value.getExpression(),out); out.add(event(d,"return","throw (exceptional path ends; handler transfer not simulated)",tree)); return true; }
            if(tree instanceof IfTree branch) {
                expression(d,branch.getCondition(),out);
                List<Object> yes=new ArrayList<>(),no=new ArrayList<>();
                boolean left=statement(d,branch.getThenStatement(),yes),right=statement(d,branch.getElseStatement(),no);
                Map<String,Object> e=event(d,"alt",text(d,branch.getCondition()),tree); e.put("children",yes); e.put("else",no); e.put("exhaustive",branch.getElseStatement()!=null); out.add(e);
                return branch.getElseStatement()!=null && left && right;
            }
            if(tree instanceof WhileLoopTree loop) {
                List<Object> body=new ArrayList<>(); expression(d,loop.getCondition(),body); statement(d,loop.getStatement(),body);
                Map<String,Object> e=event(d,"loop","while "+text(d,loop.getCondition()),tree); e.put("children",body); out.add(e); return false;
            }
            if(tree instanceof DoWhileLoopTree loop) {
                List<Object> body=new ArrayList<>(); boolean stop=statement(d,loop.getStatement(),body); if(!stop) expression(d,loop.getCondition(),body);
                Map<String,Object> e=event(d,"loop","do/while "+text(d,loop.getCondition()),tree); e.put("children",body); out.add(e); return false;
            }
            if(tree instanceof ForLoopTree loop) {
                for(StatementTree init:loop.getInitializer()) statement(d,init,out);
                List<Object> body=new ArrayList<>(); expression(d,loop.getCondition(),body); boolean stop=statement(d,loop.getStatement(),body);
                if(!stop) for(ExpressionStatementTree update:loop.getUpdate()) expression(d,update.getExpression(),body);
                Map<String,Object> e=event(d,"loop","for (condition and update repeated; iteration count unknown)",tree); e.put("children",body); out.add(e); return false;
            }
            if(tree instanceof EnhancedForLoopTree loop) {
                expression(d,loop.getExpression(),out); Map<String,Object> e=event(d,"loop","enhanced for (implicit iterator operations not expanded)",tree);
                e.put("children",region(d,loop.getStatement())); out.add(e); return false;
            }
            if(tree instanceof SwitchTree branch) { expression(d,branch.getExpression(),out); switchCases(d,tree,branch.getCases(),out); return false; }
            if(tree instanceof TryTree attempt) {
                // Resource closing, handler selection and finally transfers are not a linear trace.
                Map<String,Object> e=event(d,"unsupported","try/resource/catch/finally control transfer barrier; regions are not sequential execution",tree);
                List<Object> branches=new ArrayList<>();
                List<Object> main=new ArrayList<>(); for(Tree resource:attempt.getResources()) statement(d,resource,main); statement(d,attempt.getBlock(),main);
                branches.add(obj("label","try/resources", "events",main));
                for(CatchTree handler:attempt.getCatches()) branches.add(obj("label","catch "+text(d,handler.getParameter()),"events",region(d,handler.getBlock())));
                if(attempt.getFinallyBlock()!=null) branches.add(obj("label","finally (on normal and exceptional exits)","events",region(d,attempt.getFinallyBlock())));
                e.put("branches",branches); out.add(e); diagnostic("java.unsupported","Exceptional control transfer and implicit resource close are not simulated",d.unit,tree,false); return true;
            }
            if(tree instanceof SynchronizedTree sync) {
                expression(d,sync.getExpression(),out); out.add(event(d,"note","synchronized monitor acquisition/release (scheduling not simulated)",tree)); return statement(d,sync.getBlock(),out);
            }
            if(tree instanceof EmptyStatementTree || tree instanceof ClassTree) return false;
            if(tree instanceof ExpressionTree expression) { expression(d,expression,out); return false; }
            unsupported(d,tree,out,"Unsupported control statement: "+tree.getKind()+" (path barrier)"); return true;
        }
        void switchCases(Decl d,Tree tree,List<? extends CaseTree> cases,List<Object> out) {
            Map<String,Object> e=event(d,"alt","switch alternatives (fallthrough is an explicit barrier)",tree);
            List<Object> branches=new ArrayList<>();
            for(CaseTree branch:cases) {
                List<Object> body=new ArrayList<>();
                if(branch.getStatements()!=null) {
                    boolean stop=false;
                    for(StatementTree s:branch.getStatements()) if(statement(d,s,body)) { stop=true; break; }
                    if(!stop) unsupported(d,branch,body,"Switch fallthrough is not simulated");
                } else statement(d,branch.getBody(),body);
                String label=branch.getExpressions().isEmpty()?"default": "case "+branch.getExpressions().stream().map(v->text(d,v)).reduce((a,b)->a+", "+b).orElse("");
                branches.add(obj("label",label,"events",body));
            }
            e.put("branches",branches); e.put("exhaustive",cases.stream().anyMatch(c->c.getExpressions().isEmpty())); out.add(e);
        }
        void expression(Decl d,ExpressionTree tree,List<Object> out) {
            if(tree==null || end(d.unit,tree)<0) return;
            if(tree instanceof MethodInvocationTree call) {
                ExpressionTree select=call.getMethodSelect();
                if(select instanceof MemberSelectTree member) expression(d,member.getExpression(),out);
                for(ExpressionTree argument:call.getArguments()) expression(d,argument,out);
                invoke(d,tree,element(tree),select,out); return;
            }
            if(tree instanceof NewClassTree call) {
                expression(d,call.getEnclosingExpression(),out);
                for(ExpressionTree argument:call.getArguments()) expression(d,argument,out);
                invoke(d,tree,element(tree),null,out);
                out.add(event(d,"note","Implicit field/class/instance initialization is not simulated",tree)); return;
            }
            if(tree instanceof LambdaExpressionTree) { out.add(event(d,"note","Lambda value creation; body is a separate callable and is not executed here",tree)); return; }
            if(tree instanceof MemberReferenceTree reference) {
                expression(d,reference.getQualifierExpression(),out);
                out.add(event(d,"note","Method reference value creation; referenced callable is not invoked",tree)); return;
            }
            if(tree instanceof ParenthesizedTree value) { expression(d,value.getExpression(),out); return; }
            if(tree instanceof TypeCastTree value) { expression(d,value.getExpression(),out); return; }
            if(tree instanceof InstanceOfTree value) { expression(d,value.getExpression(),out); return; }
            if(tree instanceof MemberSelectTree value) { expression(d,value.getExpression(),out); return; }
            if(tree instanceof ArrayAccessTree value) { expression(d,value.getExpression(),out); expression(d,value.getIndex(),out); return; }
            if(tree instanceof AssignmentTree value) { expression(d,value.getVariable(),out); expression(d,value.getExpression(),out); return; }
            if(tree instanceof CompoundAssignmentTree value) { expression(d,value.getVariable(),out); expression(d,value.getExpression(),out); return; }
            if(tree instanceof UnaryTree value) { expression(d,value.getExpression(),out); return; }
            if(tree instanceof BinaryTree value) {
                expression(d,value.getLeftOperand(),out);
                if(tree.getKind()==Tree.Kind.CONDITIONAL_AND || tree.getKind()==Tree.Kind.CONDITIONAL_OR) {
                    List<Object> right=new ArrayList<>(); expression(d,value.getRightOperand(),right);
                    Map<String,Object> e=event(d,"opt",text(d,value.getLeftOperand())+(tree.getKind()==Tree.Kind.CONDITIONAL_AND?" is true":" is false"),tree); e.put("children",right); out.add(e);
                } else expression(d,value.getRightOperand(),out);
                return;
            }
            if(tree instanceof ConditionalExpressionTree value) {
                expression(d,value.getCondition(),out); List<Object> yes=new ArrayList<>(),no=new ArrayList<>();
                expression(d,value.getTrueExpression(),yes); expression(d,value.getFalseExpression(),no);
                Map<String,Object> e=event(d,"alt",text(d,value.getCondition()),tree); e.put("children",yes); e.put("else",no); e.put("exhaustive",true); out.add(e); return;
            }
            if(tree instanceof NewArrayTree array) {
                for(ExpressionTree dimension:array.getDimensions()) expression(d,dimension,out);
                if(array.getInitializers()!=null) for(ExpressionTree value:array.getInitializers()) expression(d,value,out);
                return;
            }
            if(tree instanceof SwitchExpressionTree branch) { expression(d,branch.getExpression(),out); switchCases(d,tree,branch.getCases(),out); return; }
            if(tree instanceof LiteralTree || tree instanceof IdentifierTree || tree instanceof PrimitiveTypeTree || tree instanceof ParameterizedTypeTree || tree instanceof ArrayTypeTree) return;
            unsupported(d,tree,out,"Unsupported expression: "+tree.getKind()+" (evaluation barrier)");
        }
        void invoke(Decl d,ExpressionTree tree,Element bound,ExpressionTree select,List<Object> out) {
            Map<String,Object> e=event(d,"call",text(d,tree),tree);
            if(!(bound instanceof ExecutableElement method) || method.getEnclosingElement().asType().getKind()==TypeKind.ERROR) {
                e.put("dispatch","unresolved"); e.put("label","unresolved: "+text(d,tree)); out.add(e); return;
            }
            e.put("label",selector(method));
            Set<Modifier> flags=method.getModifiers();
            boolean superCall=select instanceof MemberSelectTree member && (member.getExpression().toString().equals("super") || member.getExpression().toString().endsWith(".super"));
            TypeMirror receiver=null;
            if(select instanceof MemberSelectTree member) {
                TreePath path=paths.get(member.getExpression());
                if(path!=null) receiver=trees.getTypeMirror(path);
            } else if(select!=null) {
                TreePath path=paths.get(tree);
                while(path!=null && !(path.getLeaf() instanceof ClassTree)) path=path.getParentPath();
                if(path!=null) {
                    Element enclosing=trees.getElement(path);
                    if(enclosing!=null) receiver=enclosing.asType();
                }
            }
            boolean finalReceiver=receiver instanceof DeclaredType type && type.asElement().getModifiers().contains(Modifier.FINAL);
            boolean direct=method.getKind()==ElementKind.CONSTRUCTOR || flags.contains(Modifier.STATIC) || flags.contains(Modifier.PRIVATE) || flags.contains(Modifier.FINAL) || method.getEnclosingElement().getModifiers().contains(Modifier.FINAL) || finalReceiver || superCall;
            Decl local=methods.get(method);
            if(direct) {
                e.put("dispatch",local==null?"external":"direct");
                if(local!=null) e.put("target",local.id());
            } else {
                e.put("dispatch",method.getEnclosingElement().getKind()==ElementKind.INTERFACE?"interface":"virtual");
                Set<String> candidates=new TreeSet<>();
                if(local!=null) candidates.add(local.id());
                for(TypeElement runtimeType:selectedTypes) {
                    try {
                        TypeMirror receiverType=receiver==null?method.getEnclosingElement().asType():receiver;
                        if(receiverType.getKind()==TypeKind.ERROR || !types.isSubtype(types.erasure(runtimeType.asType()),types.erasure(receiverType))) continue;
                        for(Element member:elements.getAllMembers(runtimeType)) {
                            if(!(member instanceof ExecutableElement implementation)) continue;
                            Decl candidate=methods.get(implementation);
                            if(candidate!=null && (implementation.equals(method) || elements.overrides(implementation,method,runtimeType))) candidates.add(candidate.id());
                        }
                    } catch(RuntimeException ignored) { }
                }
                e.put("candidates",new ArrayList<>(candidates));
                e.put("label",selector(method));
            }
            out.add(e);
        }
        Object program() throws Exception {
            List<Object> files=new ArrayList<>(),callables=new ArrayList<>();
            for(Input input:inputs) files.add(obj("path",input.path,"hash",HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(input.text.getBytes(StandardCharsets.UTF_8)))));
            for(Decl d:declarations) callables.add(d.model);
            return obj("language","java","backend","jdk-javac","files",files,"callables",callables,"diagnostics",diagnostics);
        }
    }
    /** Reverse construction keeps every transfer target explicit, including loop
     * updates and labeled targets. Unsupported nodes deliberately have no edge. */
    static class Activity {
        final Engine engine; final Decl d; final long maxNodes;
        final List<Object> nodes=new ArrayList<>(), edges=new ArrayList<>();
        int entry, exit;
        static class Limit extends RuntimeException { }
        static class Scope {
            final Scope parent; final String label; final int stop, again;
            final boolean unlabeledBreak;
            Scope(Scope parent,String label,int stop,int again,boolean unlabeledBreak) {
                this.parent=parent; this.label=label; this.stop=stop; this.again=again; this.unlabeledBreak=unlabeledBreak;
            }
        }
        Activity(Engine engine,Decl d,long maxNodes) { this.engine=engine; this.d=d; this.maxNodes=maxNodes; }
        Object build() {
            engine.diagnostic("activity.intraprocedural","Calls are atomic actions; callee bodies, implicit exceptions, implicit initialization and scheduling are not expanded",d.unit,d.tree,false);
            try {
                entry=node("entry","entry",d.tree); exit=node("exit","exit",d.tree);
                int first=statement(d.body,exit,null,null);
                edge(entry,first,"normal","");
                return obj("nodes",nodes,"edges",edges,"entry",entry,"exit",exit,"diagnostics",engine.diagnostics);
            } catch(Limit limit) {
                return obj("limit",true,"diagnostics",engine.diagnostics);
            }
        }
        int node(String kind,String label,Tree tree) {
            if(nodes.size()>=maxNodes) throw new Limit();
            int id=nodes.size()+1;
            nodes.add(obj("id",id,"kind",kind,"label",label,"file",d.source.path,"span",engine.span(d.unit,tree)));
            return id;
        }
        void edge(int from,int to,String kind,String label) { edges.add(obj("from",from,"to",to,"kind",kind,"label",label)); }
        int action(Tree tree,int next) {
            int id=node("action",engine.text(d,tree),tree); edge(id,next,"normal",""); return id;
        }
        int barrier(Tree tree,String reason) {
            engine.diagnostic("activity.unsupported",reason,d.unit,tree,false);
            return node("unsupported",reason,tree);
        }
        int block(List<? extends Tree> statements,int next,Scope scope) {
            for(int i=statements.size()-1;i>=0;i--) next=statement(statements.get(i),next,scope,null);
            return next;
        }
        int condition(ExpressionTree tree,int yes,int no) {
            if(tree instanceof ParenthesizedTree p) return condition(p.getExpression(),yes,no);
            if(tree instanceof UnaryTree u && tree.getKind()==Tree.Kind.LOGICAL_COMPLEMENT) return condition(u.getExpression(),no,yes);
            if(tree instanceof BinaryTree b && tree.getKind()==Tree.Kind.CONDITIONAL_AND)
                return condition(b.getLeftOperand(),condition(b.getRightOperand(),yes,no),no);
            if(tree instanceof BinaryTree b && tree.getKind()==Tree.Kind.CONDITIONAL_OR)
                return condition(b.getLeftOperand(),yes,condition(b.getRightOperand(),yes,no));
            if(tree instanceof ConditionalExpressionTree c)
                return condition(c.getCondition(),condition(c.getTrueExpression(),yes,no),condition(c.getFalseExpression(),yes,no));
            int id=node("decision",engine.text(d,tree),tree);
            if(tree instanceof LiteralTree literal && literal.getValue() instanceof Boolean value) {
                edge(id,value?yes:no,value?"true":"false",value.toString());
            } else {
                edge(id,yes,"true","true"); edge(id,no,"false","false");
            }
            return expression(tree,id);
        }
        int transfer(Tree tree,Scope scope,String label,boolean continuing) {
            for(Scope s=scope;s!=null;s=s.parent) {
                boolean matches=label==null?(continuing?s.again>=0:s.unlabeledBreak):label.equals(s.label);
                if(matches) {
                    int target=continuing?s.again:s.stop;
                    if(target<0) break;
                    int id=node("action",engine.text(d,tree),tree);
                    edge(id,target,continuing?"continue":"break",label==null?"":label); return id;
                }
            }
            return barrier(tree,"Unresolved "+(continuing?"continue":"break")+" target (path barrier)");
        }
        int statement(Tree tree,int next,Scope scope,String label) {
            if(tree==null) return next;
            if(engine.end(d.unit,tree)<0) return next; // compiler-generated constructor/super
            if(tree instanceof BlockTree b) return block(b.getStatements(),next,scope);
            if(tree instanceof EmptyStatementTree || tree instanceof ClassTree) return next;
            if(tree instanceof LabeledStatementTree labeled) {
                String name=labeled.getLabel().toString();
                Scope named=new Scope(scope,name,next,-1,false);
                return statement(labeled.getStatement(),next,named,name);
            }
            if(tree instanceof ExpressionStatementTree e) return expression(e.getExpression(),next);
            if(tree instanceof VariableTree v) return expression(v.getInitializer(),action(tree,next));
            if(tree instanceof ReturnTree r) {
                int id=node("return",engine.text(d,tree),tree); edge(id,exit,"return","");
                return expression(r.getExpression(),id);
            }
            if(tree instanceof ThrowTree t) {
                int id=node("throw",engine.text(d,tree),tree); edge(id,exit,"throw","");
                return expression(t.getExpression(),id);
            }
            if(tree instanceof BreakTree b) return transfer(tree,scope,b.getLabel()==null?null:b.getLabel().toString(),false);
            if(tree instanceof ContinueTree c) return transfer(tree,scope,c.getLabel()==null?null:c.getLabel().toString(),true);
            if(tree instanceof IfTree branch) {
                int join=node("merge","if merge",tree); edge(join,next,"normal","");
                int yes=statement(branch.getThenStatement(),join,scope,null);
                int no=statement(branch.getElseStatement(),join,scope,null);
                return condition(branch.getCondition(),yes,no);
            }
            if(tree instanceof WhileLoopTree loop) {
                int head=node("merge","while condition",tree);
                int back=node("merge","while back",tree); edge(back,head,"back","");
                Scope inner=new Scope(scope,label,next,head,true);
                int body=statement(loop.getStatement(),back,inner,null);
                edge(head,condition(loop.getCondition(),body,next),"normal","");
                return head;
            }
            if(tree instanceof DoWhileLoopTree loop) {
                int head=node("merge","do body",tree), test=node("merge","do condition",tree);
                int back=node("merge","do back",tree); edge(back,head,"back","");
                Scope inner=new Scope(scope,label,next,test,true);
                edge(head,statement(loop.getStatement(),test,inner,null),"normal","");
                edge(test,condition(loop.getCondition(),back,next),"normal","");
                return head;
            }
            if(tree instanceof ForLoopTree loop) {
                int head=node("merge","for condition",tree), update=node("merge","for update",tree);
                int back=node("merge","for back",tree); edge(back,head,"back","");
                int updates=back;
                for(int i=loop.getUpdate().size()-1;i>=0;i--) updates=expression(loop.getUpdate().get(i).getExpression(),updates);
                edge(update,updates,"normal","");
                Scope inner=new Scope(scope,label,next,update,true);
                int body=statement(loop.getStatement(),update,inner,null);
                edge(head,loop.getCondition()==null?body:condition(loop.getCondition(),body,next),"normal","");
                return block(loop.getInitializer(),head,scope);
            }
            if(tree instanceof EnhancedForLoopTree loop) {
                int head=node("decision","enhanced for: next element?",tree);
                int back=node("merge","enhanced for back",tree); edge(back,head,"back","");
                Scope inner=new Scope(scope,label,next,head,true);
                int bind=action(loop.getVariable(),statement(loop.getStatement(),back,inner,null));
                edge(head,bind,"true","next element"); edge(head,next,"false","exhausted");
                engine.diagnostic("activity.enhanced-for","Enhanced-for iterator/array operations are represented abstractly; their implicit exceptions are not expanded",d.unit,tree,false);
                return expression(loop.getExpression(),head);
            }
            if(tree instanceof SwitchTree branch) {
                for(CaseTree c:branch.getCases()) {
                    // Later releases add pattern labels and combined null/default
                    // labels. They require matching/binding semantics beyond this
                    // Java-17 constant-label model, not a guessed default edge.
                    if((c.getExpressions().isEmpty() && !engine.text(d,c).startsWith("default")) ||
                       c.getLabels().size()!=Math.max(1,c.getExpressions().size()))
                        return barrier(tree,"Pattern/combined switch labels are unsupported (path barrier)");
                }
                Scope inner=new Scope(scope,label,next,-1,true);
                int dispatch=node("decision","switch "+engine.text(d,branch.getExpression()),tree);
                int following=next; boolean hasDefault=false;
                List<? extends CaseTree> cases=branch.getCases();
                for(int i=cases.size()-1;i>=0;i--) {
                    CaseTree c=cases.get(i);
                    int first;
                    if(c.getCaseKind()==CaseTree.CaseKind.RULE) {
                        first=statement(c.getBody(),next,inner,null);
                    } else {
                        int fall=node("merge",i+1<cases.size()?"switch fallthrough":"switch exit",c);
                        edge(fall,following,i+1<cases.size()?"fallthrough":"normal","");
                        first=block(c.getStatements(),fall,inner);
                    }
                    if(c.getExpressions().isEmpty()) {
                        hasDefault=true; edge(dispatch,first,"default","default");
                    } else for(ExpressionTree value:c.getExpressions()) edge(dispatch,first,"case",engine.text(d,value));
                    following=first;
                }
                if(!hasDefault) edge(dispatch,next,"default","no matching case");
                return expression(branch.getExpression(),dispatch);
            }
            if(tree instanceof TryTree) return barrier(tree,"try/resource/catch/finally control transfer barrier");
            if(tree instanceof SynchronizedTree) return barrier(tree,"synchronized monitor acquisition/release control transfer barrier");
            if(tree instanceof ExpressionTree e) return expression(e,next);
            return barrier(tree,"Unsupported control statement: "+tree.getKind()+" (path barrier)");
        }
        int expressions(List<? extends ExpressionTree> values,int next) {
            for(int i=values.size()-1;i>=0;i--) next=expression(values.get(i),next);
            return next;
        }
        int expression(ExpressionTree tree,int next) {
            if(tree==null || engine.end(d.unit,tree)<0) return next;
            if(tree instanceof ParenthesizedTree p) return expression(p.getExpression(),next);
            if(tree instanceof ConditionalExpressionTree c)
                return condition(c.getCondition(),expression(c.getTrueExpression(),next),expression(c.getFalseExpression(),next));
            if(tree instanceof BinaryTree b) {
                if(tree.getKind()==Tree.Kind.CONDITIONAL_AND || tree.getKind()==Tree.Kind.CONDITIONAL_OR)
                    return condition(tree,next,next);
                return expression(b.getLeftOperand(),expression(b.getRightOperand(),next));
            }
            if(tree instanceof MethodInvocationTree call) {
                int first=expressions(call.getArguments(),action(tree,next));
                if(call.getMethodSelect() instanceof MemberSelectTree member) first=expression(member.getExpression(),first);
                return first;
            }
            if(tree instanceof NewClassTree call) {
                // The anonymous class body is a declaration, never an inline call.
                return expression(call.getEnclosingExpression(),expressions(call.getArguments(),action(tree,next)));
            }
            if(tree instanceof LambdaExpressionTree) return action(tree,next);
            if(tree instanceof MemberReferenceTree r) return expression(r.getQualifierExpression(),action(tree,next));
            if(tree instanceof AssignmentTree a) return expression(a.getVariable(),expression(a.getExpression(),action(tree,next)));
            if(tree instanceof CompoundAssignmentTree a) return expression(a.getVariable(),expression(a.getExpression(),action(tree,next)));
            if(tree instanceof UnaryTree u) {
                boolean mutates=switch(tree.getKind()) {
                    case PREFIX_INCREMENT,PREFIX_DECREMENT,POSTFIX_INCREMENT,POSTFIX_DECREMENT -> true;
                    default -> false;
                };
                return expression(u.getExpression(),mutates?action(tree,next):next);
            }
            if(tree instanceof TypeCastTree c) return expression(c.getExpression(),next);
            if(tree instanceof InstanceOfTree i) return expression(i.getExpression(),next);
            if(tree instanceof MemberSelectTree m) return expression(m.getExpression(),next);
            if(tree instanceof ArrayAccessTree a) return expression(a.getExpression(),expression(a.getIndex(),next));
            if(tree instanceof NewArrayTree a) {
                int first=action(tree,next);
                if(a.getInitializers()!=null) first=expressions(a.getInitializers(),first);
                return expressions(a.getDimensions(),first);
            }
            if(tree instanceof LiteralTree || tree instanceof IdentifierTree || tree instanceof PrimitiveTypeTree ||
               tree instanceof ArrayTypeTree || tree instanceof ParameterizedTypeTree) return next;
            if(tree instanceof SwitchExpressionTree) return barrier(tree,"Switch expression/yield control transfer barrier");
            return barrier(tree,"Unsupported expression: "+tree.getKind()+" (evaluation barrier)");
        }
    }
    static void json(Object value,StringBuilder out) {
        if(value==null) { out.append("null"); return; }
        if(value instanceof String string) {
            out.append('"');
            for(int i=0;i<string.length();i++) {
                char c=string.charAt(i);
                switch(c) {
                    case '"': out.append("\\\""); break;
                    case '\\': out.append("\\\\"); break;
                    case '\n': out.append("\\n"); break;
                    case '\r': out.append("\\r"); break;
                    case '\t': out.append("\\t"); break;
                    default: if(c<32 || Character.isSurrogate(c)) out.append(String.format(Locale.ROOT,"\\u%04x",(int)c)); else out.append(c);
                }
            }
            out.append('"'); return;
        }
        if(value instanceof Map<?,?> map) {
            out.append('{'); boolean comma=false;
            for(var entry:map.entrySet()) { if(comma) out.append(','); comma=true; json(entry.getKey(),out); out.append(':'); json(entry.getValue(),out); }
            out.append('}'); return;
        }
        if(value instanceof Iterable<?> list) {
            out.append('['); boolean comma=false;
            for(Object item:list) { if(comma) out.append(','); comma=true; json(item,out); } out.append(']'); return;
        }
        out.append(value);
    }
}
