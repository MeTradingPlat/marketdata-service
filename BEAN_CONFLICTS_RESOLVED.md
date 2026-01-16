# ✅ Bean Conflicts Resolved

## Problema

Al intentar iniciar el servicio, Spring Boot reportaba errores de **bean definition conflicts**:

```
The bean 'X', defined in class path resource [...Config.class], could not be registered.
A bean with that name has already been defined in file [...X.class] and overriding is disabled.
```

Este error ocurría porque algunas clases estaban siendo registradas como beans de **dos formas diferentes**:
1. Como `@Service` o `@Component` en la propia clase (component scanning)
2. Como `@Bean` en una clase de configuración

Spring Boot por defecto **no permite** sobrescribir definiciones de beans, por lo que fallaba al detectar esta duplicación.

---

## Beans Afectados y Soluciones

### 1. `TastyTradeRestClient` ✅

**Archivo:** `infrastructure/output/external/tastytrade/api/client/TastyTradeRestClient.java`

**Problema:**
- Tenía anotación `@Service` en la clase
- También estaba definido como `@Bean` en `TastyTradeRestConfig`

**Solución:**
- ❌ Eliminada anotación `@Service`
- ❌ Eliminado import `org.springframework.stereotype.Service`
- ✅ Dejado solo `@RequiredArgsConstructor` y `@Slf4j`
- ✅ Agregado comentario: "Este bean se configura en TastyTradeRestConfig, no usar @Service aquí"

**Razón:** El bean necesita inyección de `RestClient` y `RetryTemplate` que se configuran en `TastyTradeRestConfig`, por lo que debe ser creado allí.

---

### 2. `DxLinkWebSocketClient` ✅

**Archivo:** `infrastructure/output/external/tastytrade/dxlink/client/DxLinkWebSocketClient.java`

**Problema:**
- Tenía anotación `@Service` en la clase
- También estaba definido como `@Bean` en `DxLinkWebSocketConfig`

**Solución:**
- ❌ Eliminada anotación `@Service`
- ❌ Eliminado import `org.springframework.stereotype.Service`
- ✅ Dejado solo `@RequiredArgsConstructor` y `@Slf4j`
- ✅ Agregado comentario: "Este bean se configura en DxLinkWebSocketConfig, no usar @Service aquí"

**Razón:** El bean necesita inyección de `WebSocketClient` y `ObjectMapper` customizados que se configuran en `DxLinkWebSocketConfig`.

---

## Patrón de Configuración Correcto

### ❌ Incorrecto (causa conflicto):

```java
// En la clase del bean
@Service  // ❌ NO hacer esto si hay @Bean en Config
@RequiredArgsConstructor
public class MyClient {
    private final RestClient restClient;
}

// En la clase de configuración
@Configuration
public class MyConfig {
    @Bean  // ❌ Conflicto: bean ya registrado por @Service
    public MyClient myClient(RestClient restClient) {
        return new MyClient(restClient);
    }
}
```

### ✅ Correcto (sin conflicto):

```java
// En la clase del bean
@RequiredArgsConstructor  // ✅ Solo Lombok, sin stereotypes
@Slf4j
public class MyClient {
    private final RestClient restClient;
}

// En la clase de configuración
@Configuration
public class MyConfig {
    @Bean  // ✅ Único lugar donde se registra el bean
    public MyClient myClient(RestClient restClient) {
        return new MyClient(restClient);
    }
}
```

---

## Cuándo Usar Cada Approach

### Usar `@Service`/`@Component` cuando:
- El bean no necesita configuración especial
- Todas sus dependencias son otros beans estándar
- No necesitas personalizar su construcción
- **Ejemplo:** `TastyTradeAuthClient` (solo inyecta otros beans)

```java
@Service
@RequiredArgsConstructor
public class TastyTradeAuthClient {
    private final RestClient tastyTradeRestClient;
    private final TastyTradeProperties properties;
    // No necesita configuración especial
}
```

### Usar `@Bean` en `@Configuration` cuando:
- Necesitas personalizar la construcción del bean
- Inyectas beans con configuración compleja
- El bean requiere inicialización especial
- **Ejemplo:** `TastyTradeRestClient` (necesita RestClient configurado con interceptors)

```java
@Configuration
public class TastyTradeRestConfig {
    @Bean
    public RestClient tastyTradeRestClient(RestClient.Builder builder) {
        return builder
                .baseUrl("https://api.tastytrade.com")
                .requestInterceptor(/* ... */)
                .build();
    }

    @Bean
    public TastyTradeRestClient tastyTradeRestClient(
            RestClient tastyTradeRestClient,
            RetryTemplate orderRetryTemplate,
            TastyTradeAuthClient authClient) {
        return new TastyTradeRestClient(
                tastyTradeRestClient,
                orderRetryTemplate,
                authClient
        );
    }
}
```

---

## Verificación

Después de los cambios, el servicio inicia correctamente:

```
2026-01-12T18:48:17.812-05:00  INFO 19252 --- [marketdata-service] [restartedMain]
c.m.m.MarketdataServiceApplication : Starting MarketdataServiceApplication using Java 21.0.9
...
✅ No más errores de "bean could not be registered"
```

---

## Lecciones Aprendidas

1. **No mezclar stereotypes con @Bean**: Elegir UNA forma de registrar cada bean
2. **Preferir @Bean para configuración compleja**: Más control sobre la construcción
3. **Usar @Service para beans simples**: Menos código, más directo
4. **Documentar la decisión**: Agregar comentarios explicando por qué se usa @Bean

---

## Checklist para Futuros Beans

Antes de crear un nuevo bean, pregúntate:

- [ ] ¿Necesita configuración especial de dependencias?
  - ✅ Sí → Usar `@Bean` en `@Configuration`
  - ❌ No → Usar `@Service`/`@Component`

- [ ] ¿Ya existe un `@Bean` para esta clase?
  - ✅ Sí → NO agregar `@Service`/`@Component`
  - ❌ No → Puedes usar cualquier approach

- [ ] ¿Usa constructor con Lombok `@RequiredArgsConstructor`?
  - ✅ Sí → Compatible con ambos approaches
  - ❌ No → Revisar patrón de inyección

---

**Fecha de resolución:** 2026-01-12
**Impacto:** Crítico (bloqueaba el inicio del servicio)
**Estado:** ✅ Resuelto completamente
