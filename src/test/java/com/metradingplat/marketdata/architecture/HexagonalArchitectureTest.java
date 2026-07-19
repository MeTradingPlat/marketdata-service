package com.metradingplat.marketdata.architecture;

import com.tngtech.archunit.core.domain.JavaClasses;
import com.tngtech.archunit.core.importer.ClassFileImporter;
import com.tngtech.archunit.lang.ArchRule;
import com.tngtech.archunit.lang.syntax.ArchRuleDefinition;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.stereotype.Component;
import org.springframework.stereotype.Controller;
import org.springframework.stereotype.Repository;
import org.springframework.stereotype.Service;
import org.springframework.web.bind.annotation.RestController;

import static com.tngtech.archunit.lang.syntax.ArchRuleDefinition.classes;
import static com.tngtech.archunit.lang.syntax.ArchRuleDefinition.noClasses;
import static com.tngtech.archunit.library.Architectures.layeredArchitecture;

@DisplayName("Hexagonal Architecture Tests")
class HexagonalArchitectureTest {

    private static JavaClasses importedClasses;

    @BeforeAll
    static void setUp() {
        importedClasses = new ClassFileImporter()
                .importPackages("com.metradingplat.marketdata");
    }

    @Test
    @DisplayName("Domain layer should have no dependencies on other layers")
    void domainLayerShouldHaveNoDependenciesOnOtherLayers() {
        ArchRule rule = noClasses()
                .that()
                .resideInAPackage("com.metradingplat.marketdata.domain..")
                .should()
                .dependOnClassesThat()
                .resideInAnyPackage(
                        "com.metradingplat.marketdata.application..",
                        "com.metradingplat.marketdata.adapter..",
                        "com.metradingplat.marketdata.infrastructure..",
                        "com.metradingplat.marketdata.configuration.."
                );

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Domain layer should not have Spring annotations")
    void domainLayerShouldNotHaveSpringAnnotations() {
        ArchRule rule = classes()
                .that()
                .resideInAPackage("com.metradingplat.marketdata.domain..")
                .should()
                .notBeAnnotatedWith(Component.class)
                .andShould()
                .notBeAnnotatedWith(Service.class)
                .andShould()
                .notBeAnnotatedWith(Repository.class)
                .andShould()
                .notBeAnnotatedWith(Controller.class)
                .andShould()
                .notBeAnnotatedWith(RestController.class);

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Application layer should depend only on domain layer")
    void applicationLayerShouldDependOnlyOnDomainLayer() {
        ArchRule rule = noClasses()
                .that()
                .resideInAPackage("com.metradingplat.marketdata.application..")
                .should()
                .dependOnClassesThat()
                .resideInAnyPackage(
                        "com.metradingplat.marketdata.adapter..",
                        "com.metradingplat.marketdata.infrastructure..",
                        "com.metradingplat.marketdata.configuration.."
                );

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Adapter layer should depend only on application and domain layers")
    void adapterLayerShouldDependOnlyOnApplicationAndDomainLayers() {
        ArchRule rule = noClasses()
                .that()
                .resideInAPackage("com.metradingplat.marketdata.adapter..")
                .should()
                .dependOnClassesThat()
                .resideInAnyPackage(
                        "com.metradingplat.marketdata.infrastructure..",
                        "com.metradingplat.marketdata.configuration.."
                );

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Infrastructure layer should depend only on application and domain layers")
    void infrastructureLayerShouldDependOnlyOnApplicationAndDomainLayers() {
        ArchRule rule = noClasses()
                .that()
                .resideInAPackage("com.metradingplat.marketdata.infrastructure..")
                .should()
                .dependOnClassesThat()
                .resideInAPackage("com.metradingplat.marketdata.adapter..");

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Configuration layer can depend on all layers")
    void configurationLayerCanDependOnAllLayers() {
        ArchRule rule = classes()
                .that()
                .resideInAPackage("com.metradingplat.marketdata.configuration..")
                .should()
                .onlyDependOnClassesThat()
                .resideInAnyPackage(
                        "com.metradingplat.marketdata..",
                        "org.springframework..",
                        "java..",
                        "javax.."
                );

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Layered architecture should be respected")
    void layeredArchitectureShouldBeRespected() {
        ArchRule rule = layeredArchitecture()
                .layer("Domain").definedBy("com.metradingplat.marketdata.domain..")
                .layer("Application").definedBy("com.metradingplat.marketdata.application..")
                .layer("Adapter").definedBy("com.metradingplat.marketdata.adapter..")
                .layer("Infrastructure").definedBy("com.metradingplat.marketdata.infrastructure..")
                .layer("Configuration").definedBy("com.metradingplat.marketdata.configuration..")
                .whereLayer("Domain").mayNotBeAccessedByAnyLayer()
                .whereLayer("Application").mayOnlyBeAccessedByLayers("Adapter", "Infrastructure", "Configuration")
                .whereLayer("Adapter").mayOnlyBeAccessedByLayers("Configuration")
                .whereLayer("Infrastructure").mayOnlyBeAccessedByLayers("Configuration")
                .whereLayer("Configuration").mayNotBeAccessedByAnyLayer();

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Domain entities should not depend on Spring")
    void domainEntitiesShouldNotDependOnSpring() {
        ArchRule rule = classes()
                .that()
                .resideInAPackage("com.metradingplat.marketdata.domain.entity..")
                .should()
                .notDependOnClassesThat()
                .resideInAPackage("org.springframework..");

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Domain ports should not depend on Spring")
    void domainPortsShouldNotDependOnSpring() {
        ArchRule rule = classes()
                .that()
                .resideInAPackage("com.metradingplat.marketdata.domain.port..")
                .should()
                .notDependOnClassesThat()
                .resideInAPackage("org.springframework..");

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Use cases should not depend on Spring")
    void useCasesShouldNotDependOnSpring() {
        ArchRule rule = classes()
                .that()
                .resideInAPackage("com.metradingplat.marketdata.application.usecase..")
                .should()
                .notDependOnClassesThat()
                .resideInAPackage("org.springframework..");

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Controllers should be in adapter layer")
    void controllersShouldBeInAdapterLayer() {
        ArchRule rule = classes()
                .that()
                .areAnnotatedWith(RestController.class)
                .or()
                .areAnnotatedWith(Controller.class)
                .should()
                .resideInAPackage("com.metradingplat.marketdata.adapter..");

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Repositories should be in infrastructure layer")
    void repositoriesShouldBeInInfrastructureLayer() {
        ArchRule rule = classes()
                .that()
                .areAnnotatedWith(Repository.class)
                .should()
                .resideInAPackage("com.metradingplat.marketdata.infrastructure..");

        rule.check(importedClasses);
    }

    @Test
    @DisplayName("Services should not be in domain layer")
    void servicesShouldNotBeInDomainLayer() {
        ArchRule rule = noClasses()
                .that()
                .areAnnotatedWith(Service.class)
                .should()
                .resideInAPackage("com.metradingplat.marketdata.domain..");

        rule.check(importedClasses);
    }
}
